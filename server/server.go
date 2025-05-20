package server

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

type BootServer struct {
	grpc        *grpc.Server
	http        *http.Server
	lnGrpc      net.Listener
	lnHTTP      net.Listener
	sslProvider SSLProvider
}

// Serve blocks until context is cancelled or a listen error occurs.
func (s *BootServer) Serve(ctx context.Context) error {
	grp, ctx := errgroup.WithContext(ctx)

	grp.Go(func() error {
		logger.Info("Starting gRPC server at", zap.String("port", s.lnGrpc.Addr().String()))
		return s.grpc.Serve(s.lnGrpc)
	})

	grp.Go(func() error {
		if s.sslProvider != nil {
			// run ACME helper concurrently
			if err := s.sslProvider.Run(ctx); err != nil && ctx.Err() == nil {
				return err
			}
		}
		// choose ServeTLS vs Serve
		if s.sslProvider != nil {
			logger.Info("Starting https server at", zap.String("port", s.lnHTTP.Addr().String()))
			return s.http.ServeTLS(s.lnHTTP, "", "")
		}

		logger.Info("Starting http server at", zap.String("port", s.lnHTTP.Addr().String()))
		return s.http.Serve(s.lnHTTP)
	})

	// Wait for ctx cancellation
	<-ctx.Done()

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.grpc.GracefulStop()
	_ = s.http.Shutdown(shutCtx)

	return grp.Wait()
}

// Shutdown is rarely needed (Serve handles it), but exposed for tests.
func (s *BootServer) Shutdown(ctx context.Context) error {
	s.grpc.GracefulStop()
	return s.http.Shutdown(ctx)
}
