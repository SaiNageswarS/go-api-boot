package server

import (
	"context"
	"net"
	"net/http"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"

	"github.com/SaiNageswarS/go-api-boot/auth"
	"github.com/SaiNageswarS/go-api-boot/logger"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type GoApiBoot struct {
	GrpcServer *grpc.Server
	WebServer  *http.Server
}

func NewGoApiBoot(corsConfig *cors.Cors, unaryInterceptor []grpc.UnaryServerInterceptor, streamInterceptor []grpc.StreamServerInterceptor) *GoApiBoot {
	boot := &GoApiBoot{}

	// get grpc server
	boot.GrpcServer = buildGrpcServer(unaryInterceptor, streamInterceptor)

	// get web server
	wrappedGrpc := GetWebProxy(boot.GrpcServer)
	boot.WebServer = buildWebServer(wrappedGrpc, corsConfig)
	return boot
}

func (g *GoApiBoot) Start(grpcPort, webPort string) {
	go func() {
		logger.Info("Starting server at", zap.String("port", grpcPort))

		if err := g.GrpcServer.Serve(getListener(grpcPort)); err != nil {
			logger.Fatal("Failed to serve", zap.Error(err))
		}
	}()

	logger.Info("Starting web server at ", zap.String("port", webPort))

	if err := g.WebServer.Serve(getListener(webPort)); err != nil {
		logger.Fatal("Failed to serve", zap.Error(err))
	}
}

func (g *GoApiBoot) Stop() {
	g.WebServer.Shutdown(context.Background())
	g.GrpcServer.GracefulStop()
}

func buildGrpcServer(unaryInterceptor []grpc.UnaryServerInterceptor, streamInterceptor []grpc.StreamServerInterceptor) *grpc.Server {
	streamServerInterceptors := append([]grpc.StreamServerInterceptor{
		grpc_ctxtags.StreamServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
		grpc_zap.StreamServerInterceptor(logger.Get()),
		grpc_auth.StreamServerInterceptor(auth.VerifyToken()),
	}, streamInterceptor...)

	unaryServerInterceptors := append([]grpc.UnaryServerInterceptor{
		grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
		grpc_zap.UnaryServerInterceptor(logger.Get()),
		grpc_auth.UnaryServerInterceptor(auth.VerifyToken()),
	}, unaryInterceptor...)

	s := grpc.NewServer(
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamServerInterceptors...)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryServerInterceptors...)),
	)

	return s
}

func buildWebServer(wrappedGrpc http.Handler, corsConfig *cors.Cors) *http.Server {
	serveMux := http.NewServeMux()
	serveMux.Handle("/", corsConfig.Handler(wrappedGrpc))
	serveMux.Handle("/metrics", promhttp.Handler())
	serveMux.HandleFunc("/health", func(resp http.ResponseWriter, req *http.Request) {
		resp.WriteHeader(http.StatusOK)
	})

	return &http.Server{
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
		Handler:      serveMux,
	}
}

func getListener(port string) net.Listener {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		logger.Fatal("Failed to listen", zap.Error(err))
	}

	return lis
}
