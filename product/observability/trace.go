package observability

import (
	"context"
	"fmt" // Added for fmt.Errorf

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

// Span is a wrapper around trace.Span that automatically manages context restoration.
type Span struct {
	trace.Span
	obs       *Observability
	parentCtx context.Context
}

// End restores the parent context in the Observability instance and then ends the span.
func (s *Span) End(options ...trace.SpanEndOption) {
	s.obs.SetContext(s.parentCtx)
	s.Span.End(options...)
}

// Trace wraps the OpenTelemetry tracer.
type Trace struct {
	obs    *Observability
	tracer trace.Tracer
}

// NewTrace creates a new Trace instance.
func NewTrace(obs *Observability, serviceName string) *Trace {
	return &Trace{
		obs:    obs,
		tracer: otel.Tracer(serviceName),
	}
}

// Start creates a new span and updates the context in the Observability instance.
func (t *Trace) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, *Span) {
	// We take the context from the obs instance as the parent context.
	parentCtx := t.obs.Context()
	newCtx, otelSpan := t.tracer.Start(ctx, spanName, opts...)
	t.obs.SetContext(newCtx) // Set the new context for the duration of the span.

	// The returned span wrapper holds the original parent context.
	span := &Span{
		Span:      otelSpan,
		obs:       t.obs,
		parentCtx: parentCtx,
	}
	return newCtx, span
}

// SetupTracing initializes and configures the global TracerProvider based on APM type.
func SetupTracing(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, apmType string) (Shutdowner, error) {
	switch APMType(apmType) {
	case OTLP:
		return setupOTLP(ctx, serviceName, serviceApp, serviceEnv, apmURL)
	default:
		return nil, fmt.Errorf("unsupported APM type: %s", apmType)
	}
}

// setupOTLP configures and initializes the OpenTelemetry TracerProvider.
func setupOTLP(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string) (*sdktrace.TracerProvider, error) {
	exporter, err := newOTLPExporter(ctx, apmURL)
	if err != nil {
		return nil, err
	}

	// Configure TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter), // Send traces in batches
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
			attribute.String("application", serviceApp),
			attribute.String("environment", serviceEnv),
		)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // Always sample all traces
	)

	// Set TracerProvider global
	otel.SetTracerProvider(tp)

	// Set global propagator for HTTP headers
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, // W3C Trace Context (standard for distributed tracing)
		propagation.Baggage{},
	))

	// Use a temporary observability instance for this setup log.
	obs := NewObservability(ctx, serviceName)
	obs.Log.Info("OpenTelemetry TracerProvider initialized successfully",
		"APMURL", apmURL,
		"APMType", OTLP,
	)

	return tp, nil
}

// newOTLPExporter creates a new OTLP exporter.
func newOTLPExporter(ctx context.Context, apmURL string) (sdktrace.SpanExporter, error) {
	// Create OTLP HTTP Exporter
	client := otlptracehttp.NewClient(
		otlptracehttp.WithEndpointURL(apmURL),
	)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}
	return exporter, nil
}