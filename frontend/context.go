package main

import (
	"context"
	"frontend-service/observability"
)

// obsKey is a private type to prevent collisions with other packages
type obsKey struct{}

// CtxWithObs returns a new context with the Observability instance stored.
func CtxWithObs(ctx context.Context, obs *observability.Observability) context.Context {
	return context.WithValue(ctx, obsKey{}, obs)
}

// ObsFromCtx retrieves the Observability instance from the context.
// It returns a new Observability instance with a nil logger if not found.
func ObsFromCtx(ctx context.Context) *observability.Observability {
	if obs, ok := ctx.Value(obsKey{}).(*observability.Observability); ok {
		return obs
	}
	// Return a new Observability instance with a nil logger to avoid panics
	return &observability.Observability{}
}
