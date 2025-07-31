
package observability

import (
	"context"
	"log/slog"
)

// Observability holds the tracing and logging components.
type Observability struct {
	Trace *Trace
	Log   *Log
	ctx   context.Context
}

// NewObservability creates a new Observability instance.
func NewObservability(ctx context.Context, baseLogger *slog.Logger, serviceName string) *Observability {
	obs := &Observability{
		ctx: ctx,
	}
	obs.Trace = NewTrace(obs, serviceName) // Pass obs to Trace
	obs.Log = NewLog(obs, baseLogger)      // Pass obs to Log
	return obs
}

// Context returns the current context from the Observability instance.
func (o *Observability) Context() context.Context {
	return o.ctx
}

// SetContext updates the context in the Observability instance.
func (o *Observability) SetContext(ctx context.Context) {
	o.ctx = ctx
}
