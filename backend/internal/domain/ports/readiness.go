// Package ports declares the contracts the application depends on.
package ports

import "context"

// ReadinessCheck reports whether one dependency is able to serve traffic.
type ReadinessCheck interface {
	Name() string
	Check(ctx context.Context) error
}
