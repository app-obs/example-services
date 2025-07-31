package observability

import (
	"context"
	"fmt"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

// Span is an interface for a trace span.
type Span interface {
	End()
	AddEvent(string, ...trace.EventOption)
	RecordError(error, ...trace.EventOption)
	SetStatus(codes.Code, string)
}

// Tracer is an interface for a tracer.
type Tracer interface {
	Start(ctx context.Context, spanName string) (context.Context, Span)
}

// OTelSpan is a wrapper around trace.Span that automatically manages context restoration.
type OTelSpan struct {
	trace.Span
	obs       *Observability
	parentCtx context.Context
}

// End restores the parent context in the Observability instance and then ends the span.
func (s *OTelSpan) End() {
	s.obs.SetContext(s.parentCtx)
	s.Span.End()
}

// OTelTracer wraps the OpenTelemetry tracer.
type OTelTracer struct {
	obs    *Observability
	tracer trace.Tracer
}

// Start creates a new span and updates the context in the Observability instance.
func (t *OTelTracer) Start(ctx context.Context, spanName string) (context.Context, Span) {
	parentCtx := t.obs.Context()
	newCtx, otelSpan := t.tracer.Start(ctx, spanName)
	t.obs.SetContext(newCtx)

	span := &OTelSpan{
		Span:      otelSpan,
		obs:       t.obs,
		parentCtx: parentCtx,
	}
	return newCtx, span
}

// DataDogSpan is a wrapper around ddtrace.Span.
type DataDogSpan struct {
	tracer.Span
	obs       *Observability
	parentCtx context.Context
}

// End ends the span.
func (s *DataDogSpan) End() {
	s.obs.SetContext(s.parentCtx)
	s.Finish()
}

// AddEvent adds an event to the span.
func (s *DataDogSpan) AddEvent(name string, options ...trace.EventOption) {
	s.SetTag("event", name)
}

// RecordError records an error on the span.
func (s *DataDogSpan) RecordError(err error, options ...trace.EventOption) {
	s.SetTag("error", err)
}

// SetStatus sets the status of the span.
func (s *DataDogSpan) SetStatus(code codes.Code, description string) {
	s.SetTag("status", description)
}

// DataDogTracer wraps the DataDog tracer.
type DataDogTracer struct {
	obs *Observability
}

// Start creates a new span.
func (t *DataDogTracer) Start(ctx context.Context, spanName string) (context.Context, Span) {
	parentCtx := t.obs.Context()
	span, newCtx := tracer.StartSpanFromContext(ctx, spanName)
	t.obs.SetContext(newCtx)
	return newCtx, &DataDogSpan{
		Span:      span,
		obs:       t.obs,
		parentCtx: parentCtx,
	}
}

// Trace holds the active tracer.
type Trace struct {
	Tracer
}

// NewTrace creates a new Trace instance.
func NewTrace(obs *Observability, serviceName string, apmType APMType) *Trace {
	var t Tracer
	switch apmType {
	case OTLP:
		t = &OTelTracer{
			obs:    obs,
			tracer: otel.Tracer(serviceName),
		}
	case DataDog:
		t = &DataDogTracer{
			obs: obs,
		}
	}
	return &Trace{
		Tracer: t,
	}
}

// SetupTracing initializes and configures the global TracerProvider based on APM type.
func SetupTracing(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, apmType string) (Shutdowner, error) {
	switch APMType(apmType) {
	case OTLP:
		return setupOTLP(ctx, serviceName, serviceApp, serviceEnv, apmURL)
	case DataDog:
		return setupDataDog(ctx, serviceName, serviceApp, serviceEnv, apmURL)
	default:
		return nil, fmt.Errorf("unsupported APM type: %s", apmType)
	}
}

// setupDataDog configures and initializes the DataDog Tracer.
func setupDataDog(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string) (Shutdowner, error) {
	tracer.Start(
		tracer.WithService(serviceName),
		tracer.WithEnv(serviceEnv),
		tracer.WithServiceVersion(serviceApp),
		tracer.WithAgentAddr(apmURL),
	)

	obs := NewObservability(ctx, serviceName, DataDog)
	obs.Log.Info("DataDog Tracer initialized successfully",
		"APMURL", apmURL,
		"APMType", DataDog,
	)

	return &dataDogShutdowner{}, nil
}

// dataDogShutdowner implements the Shutdowner interface for DataDog.
type dataDogShutdowner struct{}

// Shutdown stops the DataDog tracer.
func (d *dataDogShutdowner) Shutdown(ctx context.Context) error {
	tracer.Stop()
	return nil
}

// setupOTLP configures and initializes the OpenTelemetry TracerProvider.
func setupOTLP(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string) (Shutdowner, error) {
	exporter, err := newOTLPExporter(ctx, apmURL)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
			attribute.String("application", serviceApp),
			attribute.String("environment", serviceEnv),
		)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	obs := NewObservability(ctx, serviceName, OTLP)
	obs.Log.Info("OpenTelemetry TracerProvider initialized successfully",
		"APMURL", apmURL,
		"APMType", OTLP,
	)

	return tp, nil
}

// newOTLPExporter creates a new OTLP exporter.
func newOTLPExporter(ctx context.Context, apmURL string) (sdktrace.SpanExporter, error) {
	client := otlptracehttp.NewClient(
		otlptracehttp.WithEndpointURL(apmURL),
	)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}
	return exporter, nil
}
