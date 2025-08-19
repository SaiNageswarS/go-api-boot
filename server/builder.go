package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"time"

	"github.com/SaiNageswarS/go-api-boot/auth"
	"github.com/SaiNageswarS/go-api-boot/logger"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// ─── public fluent builder ───────────────────────────────────
type Builder struct {
	grpcPort    string
	httpPort    string
	staticDir   string
	sslProvider SSLProvider

	unary  []grpc.UnaryServerInterceptor
	stream []grpc.StreamServerInterceptor
	cors   *cors.Cors
	extra  map[string]http.HandlerFunc

	singletons map[reflect.Type]reflect.Value
	providers  map[reflect.Type]reflect.Value
	reg        []registration

	serverOpts []grpc.ServerOption

	// temporal worker for DI
	taskQueue          string
	activityRegs       []reflect.Value
	workflowRegs       []interface{}
	temporalClientOpts *client.Options
}

type registration struct {
	register func(grpc.ServiceRegistrar, any) // generated pb.Register…Server
	factory  reflect.Value                    // user-supplied func(dep1,…)*Svc
}

func New() *Builder {
	return &Builder{
		cors:       cors.AllowAll(),
		extra:      map[string]http.HandlerFunc{},
		singletons: map[reflect.Type]reflect.Value{},
		providers:  map[reflect.Type]reflect.Value{},
		unary: []grpc.UnaryServerInterceptor{
			grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_zap.UnaryServerInterceptor(logger.Get()),
			grpc_auth.UnaryServerInterceptor(auth.VerifyToken()),
		},
		stream: []grpc.StreamServerInterceptor{
			grpc_ctxtags.StreamServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_zap.StreamServerInterceptor(logger.Get()),
			grpc_auth.StreamServerInterceptor(auth.VerifyToken()),
		},
	}
}

// ----- basic wiring ----------------------------------------------------------

func (b *Builder) GRPCPort(p string) *Builder { b.grpcPort = p; return b }
func (b *Builder) HTTPPort(p string) *Builder { b.httpPort = p; return b }

// StaticDir sets the directory to serve static files from (e.g., "./static").
// Static files will be served on the same HTTP port at /static/* path.
func (b *Builder) StaticDir(dir string) *Builder { b.staticDir = dir; return b }

func (b *Builder) EnableSSL(p SSLProvider) *Builder { b.sslProvider = p; return b }

func (b *Builder) Unary(i ...grpc.UnaryServerInterceptor) *Builder {
	b.unary = append(b.unary, i...)
	return b
}
func (b *Builder) Stream(i ...grpc.StreamServerInterceptor) *Builder {
	b.stream = append(b.stream, i...)
	return b
}
func (b *Builder) CORS(c *cors.Cors) *Builder { b.cors = c; return b }

func (b *Builder) Handle(pattern string, h http.HandlerFunc) *Builder {
	b.extra[pattern] = h
	return b
}

// ---- temporal worker -----------------------------------------------------

func (b *Builder) WithTemporal(taskQueue string, opts *client.Options) *Builder {
	b.taskQueue = taskQueue
	b.temporalClientOpts = opts
	return b
}

func (b *Builder) RegisterTemporalWorkflow(w interface{}) *Builder {
	if w == nil {
		logger.Fatal("temporal workflow factory must not be nil")
	}
	b.workflowRegs = append(b.workflowRegs, w)
	return b
}

func (b *Builder) RegisterTemporalActivity(factory any) *Builder {
	v := reflect.ValueOf(factory)
	if v.Kind() != reflect.Func {
		logger.Fatal("activity receiver factory must be a func", zap.Any("received", factory))
	}
	b.activityRegs = append(b.activityRegs, v)
	return b
}

// ----- dependency injection --------------------------------------------------

func (b *Builder) Provide(value any) *Builder {
	b.singletons[reflect.TypeOf(value)] = reflect.ValueOf(value)
	return b
}

func (b *Builder) ProvideAs(value any, ifacePtr any) *Builder {
	ifaceType := reflect.TypeOf(ifacePtr).Elem()
	val := reflect.ValueOf(value)

	if !val.Type().Implements(ifaceType) {
		logger.Fatal("Provided value does not implement the given interface",
			zap.String("valueType", val.Type().String()),
			zap.String("interfaceType", ifaceType.String()))
	}

	b.singletons[ifaceType] = val
	return b
}

func (b *Builder) ProvideFunc(fn any) *Builder {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		logger.Fatal("ProvideFunc expects a function", zap.Any("received", fn))
	}
	out := v.Type().Out(0)
	b.providers[out] = v
	return b
}

func (b *Builder) RegisterService(
	register func(grpc.ServiceRegistrar, any),
	factory any,
) *Builder {
	v := reflect.ValueOf(factory)
	if v.Kind() != reflect.Func {
		logger.Fatal("factory must be a function", zap.Any("received", factory))
	}
	b.reg = append(b.reg, registration{register, v})
	return b
}

func Adapt[S any](fn func(grpc.ServiceRegistrar, S)) func(grpc.ServiceRegistrar, any) {
	return func(r grpc.ServiceRegistrar, v any) { fn(r, v.(S)) }
}

// Apply server settings
func (b *Builder) ApplySettings(opts []grpc.ServerOption) *Builder {
	b.serverOpts = append(b.serverOpts, opts...)
	return b
}

// ----- Resolve DI and build servers/workers -----------------------------------------------------

func (b *Builder) Build() (*BootServer, error) {
	if b.grpcPort == "" || b.httpPort == "" {
		return nil, errors.New("grpc and http ports must be set")
	}

	lnGrpc, err := net.Listen("tcp", b.grpcPort)
	if err != nil {
		return nil, err
	}
	lnHTTP, err := net.Listen("tcp", b.httpPort)
	if err != nil {
		return nil, err
	}

	// Prepare server options
	b.serverOpts = append(b.serverOpts,
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(b.stream...)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(b.unary...)),
	)

	grpcSrv := grpc.NewServer(b.serverOpts...)

	// tiny DI container
	ctn := newContainer(b.singletons, b.providers)

	// register services
	for _, r := range b.reg {
		svc, err := invokeFactory(ctn, r.factory)
		if err != nil {
			return nil, err
		}
		r.register(grpcSrv, svc.Interface())
	}

	// HTTP multiplexer
	mux := http.NewServeMux()

	webProxy := GetWebProxy(grpcSrv)
	mux.Handle("/", b.cors.Handler(webProxy))

	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// register extra handlers
	for p, h := range b.extra {
		mux.HandleFunc(p, h)
	}

	// Add static file serving if configured
	if b.staticDir != "" {
		fileServer := http.FileServer(http.Dir(b.staticDir))
		mux.Handle("/static/", http.StripPrefix("/static/", fileServer))
	}

	// HTTP server with optimized timeouts
	var readTimeout, writeTimeout, idleTimeout time.Duration
	readTimeout = 5 * time.Minute
	writeTimeout = 5 * time.Minute
	idleTimeout = 10 * time.Minute

	httpSrv := &http.Server{
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	if b.sslProvider != nil {
		if err := b.sslProvider.Configure(httpSrv); err != nil {
			return nil, err
		}
	}

	// Create a temporal worker if configured
	var tw worker.Worker
	var tc client.Client
	var tcErr error
	if b.temporalClientOpts != nil {
		err := RetryWithExponentialBackoff(context.Background(), 5, 10*time.Second, func() error {
			tc, tcErr = client.Dial(*b.temporalClientOpts)
			if tcErr != nil {
				return tcErr
			}
			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("failed to create temporal client: %w", err)
		}
		tw = worker.New(tc, b.taskQueue, worker.Options{})

		for _, f := range b.activityRegs {
			receiver, err := invokeFactory(ctn, f)
			if err != nil {
				return nil, fmt.Errorf("activity DI failed: %w", err)
			}
			tw.RegisterActivity(receiver.Interface())
		}

		for _, wf := range b.workflowRegs {
			tw.RegisterWorkflow(wf)
		}
	}

	return &BootServer{
		grpc:           grpcSrv,
		http:           httpSrv,
		lnGrpc:         lnGrpc,
		lnHTTP:         lnHTTP,
		sslProvider:    b.sslProvider,
		temporalWorker: tw,
		temporalClient: tc,
	}, nil
}

// invokeFactory resolves arguments via container and calls the func.
func invokeFactory(ctn *container, fn reflect.Value) (reflect.Value, error) {
	args := make([]reflect.Value, fn.Type().NumIn())
	for i := range args {
		v, err := ctn.resolve(fn.Type().In(i))
		if err != nil {
			return v, err
		}
		args[i] = v
	}
	return fn.Call(args)[0], nil
}
