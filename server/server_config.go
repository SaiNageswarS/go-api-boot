package server

import (
	"net/http"

	"github.com/SaiNageswarS/go-api-boot/auth"
	"github.com/SaiNageswarS/go-api-boot/logger"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/rs/cors"
	"google.golang.org/grpc"
)

type Option func(*Config)

type Config struct {
	CorsConfig         *cors.Cors
	UnaryInterceptors  []grpc.UnaryServerInterceptor
	StreamInterceptors []grpc.StreamServerInterceptor

	// Additional http handlers. All of the gRpc APIs will be exposed by default on http Rest for web.
	ExtraHttpHandlers map[string]func(http.ResponseWriter, *http.Request)
	SSL               bool
}

func NewConfig(options ...Option) *Config {
	config := &Config{
		CorsConfig: cors.New(cors.Options{AllowedHeaders: []string{"*"}}),
		UnaryInterceptors: []grpc.UnaryServerInterceptor{
			grpc_ctxtags.UnaryServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_zap.UnaryServerInterceptor(logger.Get()),
			grpc_auth.UnaryServerInterceptor(auth.VerifyToken()),
		},
		StreamInterceptors: []grpc.StreamServerInterceptor{
			grpc_ctxtags.StreamServerInterceptor(grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor)),
			grpc_zap.StreamServerInterceptor(logger.Get()),
			grpc_auth.StreamServerInterceptor(auth.VerifyToken()),
		},
		ExtraHttpHandlers: map[string]func(http.ResponseWriter, *http.Request){},
	}

	for _, option := range options {
		option(config)
	}

	return config
}

func WithCorsConfig(corsConfig *cors.Cors) Option {
	return func(c *Config) {
		c.CorsConfig = corsConfig
	}
}

func AppendUnaryInterceptors(interceptors []grpc.UnaryServerInterceptor) Option {
	return func(c *Config) {
		c.UnaryInterceptors = append(c.UnaryInterceptors, interceptors...)
	}
}

func AppendStreamInterceptors(interceptors []grpc.StreamServerInterceptor) Option {
	return func(c *Config) {
		c.StreamInterceptors = append(c.StreamInterceptors, interceptors...)
	}
}

func AppendHttpHandlers(handlers map[string]func(http.ResponseWriter, *http.Request)) Option {
	return func(c *Config) {
		c.ExtraHttpHandlers = handlers
	}
}

func WithSSL(ssl bool) Option {
	return func(c *Config) {
		c.SSL = ssl
	}
}
