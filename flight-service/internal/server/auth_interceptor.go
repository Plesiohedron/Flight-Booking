package server

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthInterceptor checks that the caller provides the correct API key
// in the "x-api-key" gRPC metadata header.
type AuthInterceptor struct {
	apiKey string
}

// NewAuthInterceptor creates a new AuthInterceptor with the given expected API key.
func NewAuthInterceptor(apiKey string) *AuthInterceptor {
	return &AuthInterceptor{apiKey: apiKey}
}

// Unary returns a grpc.UnaryServerInterceptor that enforces API key authentication.
func (a *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if err := a.authorize(ctx); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func (a *AuthInterceptor) authorize(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}

	values := md.Get("x-api-key")
	if len(values) == 0 {
		return status.Error(codes.Unauthenticated, "missing x-api-key header")
	}
	if values[0] != a.apiKey {
		// *
		return status.Error(codes.Unauthenticated, "invalid api key")
	}
	return nil
}

