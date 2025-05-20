package server

import (
	"errors"
	"net"
	"net/http"
	"reflect"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"google.golang.org/grpc"

	"github.com/SaiNageswarS/go-api-boot/config"
)

// ─── public fluent builder ───────────────────────────────────
type Builder struct {
	cfg         *config.BootConfig
	grpcPort    string
	httpPort    string
	sslProvider SSLProvider

	unary  []grpc.UnaryServerInterceptor
	stream []grpc.StreamServerInterceptor
	cors   *cors.Cors
	extra  map[string]http.HandlerFunc

	singletons map[reflect.Type]reflect.Value
	providers  map[reflect.Type]reflect.Value
	reg        []registration
}

type registration struct {
	register func(grpc.ServiceRegistrar, any) // generated pb.Register…Server
	factory  reflect.Value                    // user-supplied func(dep1,…)*Svc
}

// New returns a fresh builder; cfg may be nil if you DI it later.
func New(cfg *config.BootConfig) *Builder {
	return &Builder{
		cfg:        cfg,
		cors:       cors.AllowAll(),
		extra:      map[string]http.HandlerFunc{},
		singletons: map[reflect.Type]reflect.Value{},
		providers:  map[reflect.Type]reflect.Value{},
	}
}

// ----- basic wiring ----------------------------------------------------------

func (b *Builder) GRPCPort(p string) *Builder { b.grpcPort = p; return b }
func (b *Builder) HTTPPort(p string) *Builder { b.httpPort = p; return b }

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

// ----- dependency injection --------------------------------------------------

func (b *Builder) Provide(value any) *Builder {
	b.singletons[reflect.TypeOf(value)] = reflect.ValueOf(value)
	return b
}

// ProvideFunc registers a lazy provider func(dep1,…,depN) T.
func (b *Builder) ProvideFunc(fn any) *Builder {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		panic("ProvideFunc expects a function")
	}
	out := v.Type().Out(0)
	b.providers[out] = v
	return b
}

// Register ties a generated pb.Register…Server with your factory func.
func (b *Builder) Register(
	register func(grpc.ServiceRegistrar, any),
	factory any,
) *Builder {
	v := reflect.ValueOf(factory)
	if v.Kind() != reflect.Func {
		panic("factory must be a function")
	}
	b.reg = append(b.reg, registration{register, v})
	return b
}

// ----- build -----------------------------------------------------------------

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

	grpcSrv := grpc.NewServer(
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(b.stream...)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(b.unary...)),
	)

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
	// Web Proxy for gRPC handlers.
	mux.Handle("/", b.cors.Handler(GetWebProxy(grpcSrv)))

	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// register extra handlers
	for p, h := range b.extra {
		mux.HandleFunc(p, h)
	}

	httpSrv := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	if b.sslProvider != nil {
		if err := b.sslProvider.Configure(httpSrv); err != nil {
			return nil, err
		}
	}

	return &BootServer{
		grpc:        grpcSrv,
		http:        httpSrv,
		lnGrpc:      lnGrpc,
		lnHTTP:      lnHTTP,
		sslProvider: b.sslProvider,
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
