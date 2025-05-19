package server

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"golang.org/x/crypto/acme/autocert"

	"github.com/SaiNageswarS/go-api-boot/cloud"
	"github.com/SaiNageswarS/go-api-boot/config"
	"github.com/SaiNageswarS/go-api-boot/logger"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type GoApiBoot struct {
	GrpcServer *grpc.Server
	WebServer  *http.Server
	ssl        bool
	cloudFns   cloud.Cloud
	config     *config.BaseConfig
}

func NewGoApiBoot(config *config.BaseConfig, options ...Option) *GoApiBoot {
	bootServerSettings := NewBootServerSettings(options...)
	boot := &GoApiBoot{config: config}

	// get grpc server
	boot.GrpcServer = buildGrpcServer(bootServerSettings.UnaryInterceptors, bootServerSettings.StreamInterceptors)

	// get web server
	wrappedGrpc := GetWebProxy(boot.GrpcServer)
	boot.WebServer = buildWebServer(wrappedGrpc, bootServerSettings.CorsConfig, bootServerSettings.ExtraHttpHandlers)
	boot.ssl = bootServerSettings.SSL
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
	if g.ssl {
		var cache autocert.Cache
		if g.cloudFns != nil {
			cache = NewSslCloudCache(g.config, g.cloudFns)
		} else {
			cache = autocert.DirCache("certs")
		}

		sslManager := NewSSLManager(cache)
		sslManager.RunAcmeChallengeListener()
		sslManager.DownloadCertificatesWithRetry()

		g.WebServer.TLSConfig = &tls.Config{GetCertificate: sslManager.certManager.GetCertificate}
		if err := g.WebServer.ServeTLS(getListener(webPort), "", ""); err != nil {
			logger.Fatal("Failed to serve", zap.Error(err))
		}
	} else {
		if err := g.WebServer.Serve(getListener(webPort)); err != nil {
			logger.Fatal("Failed to serve", zap.Error(err))
		}
	}
}

func (g *GoApiBoot) Stop() {
	g.WebServer.Shutdown(context.Background())
	g.GrpcServer.GracefulStop()
}

func buildGrpcServer(unaryInterceptor []grpc.UnaryServerInterceptor, streamInterceptor []grpc.StreamServerInterceptor) *grpc.Server {
	s := grpc.NewServer(
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamInterceptor...)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptor...)),
	)

	return s
}

func buildWebServer(wrappedGrpc http.Handler, corsConfig *cors.Cors, handlers map[string]func(http.ResponseWriter, *http.Request)) *http.Server {
	serveMux := http.NewServeMux()
	serveMux.Handle("/", corsConfig.Handler(wrappedGrpc))
	serveMux.Handle("/metrics", promhttp.Handler())
	serveMux.HandleFunc("/health", func(resp http.ResponseWriter, req *http.Request) {
		resp.WriteHeader(http.StatusOK)
	})

	addHandlersToServeMux(serveMux, handlers)

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

func addHandlersToServeMux(serveMux *http.ServeMux, handlers map[string]func(http.ResponseWriter, *http.Request)) {
	for pattern, handler := range handlers {
		serveMux.HandleFunc(pattern, handler)
	}
}
