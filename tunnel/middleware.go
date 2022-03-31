package tunnel

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type contextKey string

const (
	idContextKey = contextKey("id")
)

func id(ctx context.Context) tunnelID {
	return ctx.Value(idContextKey).(tunnelID)
}

type wrappedStream struct {
	grpc.ServerStream
	wrappedContext context.Context
}

func (s *wrappedStream) Context() context.Context {
	return s.wrappedContext
}

func wrapStream(stream grpc.ServerStream, id tunnelID) *wrappedStream {
	return &wrappedStream{
		ServerStream:   stream,
		wrappedContext: context.WithValue(stream.Context(), idContextKey, id),
	}
}

func spiffeStreamMiddleware(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if id, ok := verifySPIFFE(ss.Context()); ok {
		return handler(srv, wrapStream(ss, id))
	}
	return status.Errorf(codes.Unauthenticated, "unable to authenticate request")
}

func verifySPIFFE(ctx context.Context) (tunnelID, bool) {
	if p, ok := peer.FromContext(ctx); ok {
		if mtls, ok := p.AuthInfo.(credentials.TLSInfo); ok {
			// grab the peer certificate info
			for _, item := range mtls.State.PeerCertificates {
				// check each untyped SAN for spiffe information
				for _, uri := range item.URIs {
					if uri.Scheme == "spiffe" {
						return tunnelID{
							id:    uri.Host,
							nonce: strings.TrimPrefix(uri.Path, "/"),
						}, true
					}
				}
			}
		}
	}
	return tunnelID{}, false
}
