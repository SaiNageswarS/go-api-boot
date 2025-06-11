package server

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/SaiNageswarS/go-api-boot/logger"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

type BootServer struct {
	grpc           *grpc.Server
	http           *http.Server
	lnGrpc         net.Listener
	lnHTTP         net.Listener
	sslProvider    SSLProvider
	temporalWorker worker.Worker
	temporalClient client.Client
}

// Serve blocks until context is cancelled or a listen error occurs.
func (s *BootServer) Serve(ctx context.Context) error {
	grp, ctx := errgroup.WithContext(ctx)

	// Start gRPC server
	grp.Go(func() error {
		logger.Info("Starting gRPC server at", zap.String("port", s.lnGrpc.Addr().String()))
		return s.grpc.Serve(s.lnGrpc)
	})

	// Start HTTP server
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

	// Start Temporal worker if configured
	if s.temporalWorker != nil {
		grp.Go(func() error {
			logger.Info("Starting Temporal worker...")
			return RetryWithExponentialBackoff(ctx, 5, 10*time.Second, func() error {
				err := s.temporalWorker.Run(worker.InterruptCh())
				if err != nil {
					logger.Error("Temporal worker failed to start", zap.Error(err))
				}

				return err
			})
		})
	}

	// Wait for ctx cancellation
	<-ctx.Done()

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.grpc.GracefulStop()
	_ = s.http.Shutdown(shutCtx)
	if s.temporalClient != nil {
		s.temporalClient.Close()
	}

	return grp.Wait()
}

// Shutdown is rarely needed (Serve handles it), but exposed for tests.
func (s *BootServer) Shutdown(ctx context.Context) error {
	s.grpc.GracefulStop()
	return s.http.Shutdown(ctx)
}
