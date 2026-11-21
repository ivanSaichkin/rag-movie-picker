// Package requestid carries the request identifier through contexts, so that
// outbound calls can correlate their logs with the incoming request.
package requestid

import "context"

type contextKey struct{}

// WithID stores the request identifier in the context.
func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// FromContext returns the identifier or an empty string.
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}
