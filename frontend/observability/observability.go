package observability

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// Observability holds the tracing and logging components.
type Observability struct {
	Trace *Trace
	Log   *Log
	ctx   context.Context
}

// NewObservabilityFromRequest creates a new Observability instance by extracting the
// trace context from an incoming HTTP request.
func NewObservabilityFromRequest(r *http.Request, serviceName string) *Observability {
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	return NewObservability(ctx, serviceName)
}

// NewObservability creates a new Observability instance.
func NewObservability(ctx context.Context, serviceName string) *Observability {
	obs := &Observability{
		ctx: ctx,
	}
	baseLogger := InitLogger()
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