package server

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

var (
	GrpcRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total gRPC requests handled by flight-service.",
		},
		[]string{"method", "endpoint", "status"},
	)
	GrpcRequestErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_request_errors_total",
			Help: "Total gRPC request errors handled by flight-service.",
		},
		[]string{"method", "endpoint", "error_type"},
	)
	GrpcRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "gRPC request duration in seconds for flight-service.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)
)

func init() {
	prometheus.MustRegister(GrpcRequestsTotal, GrpcRequestErrorsTotal, GrpcRequestDurationSeconds)
}

// MetricsUnaryInterceptor records Prometheus metrics for every unary gRPC call.
func MetricsUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		started := time.Now()
		resp, err := handler(ctx, req)
		code := status.Code(err).String()
		endpoint := info.FullMethod

		GrpcRequestsTotal.WithLabelValues("gRPC", endpoint, code).Inc()
		GrpcRequestDurationSeconds.WithLabelValues("gRPC", endpoint).Observe(time.Since(started).Seconds())
		if err != nil {
			GrpcRequestErrorsTotal.WithLabelValues("gRPC", endpoint, code).Inc()
		}

		return resp, err
	}
}
