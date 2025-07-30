package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes" // Untuk span.SetStatus
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	serviceApp  = getEnvOrDefault("APPLICATION", "ecommerce")
	serviceEnv  = getEnvOrDefault("ENVIRONMENT", "development")
	collectorURL = getEnvOrDefault("OTLP_URL", "http://tempo:4318/v1/traces")

	serviceName  = getEnvOrDefault("SERVICE_NAME", "product-service")
	EnvPort      = "PORT"
	DefaultPort  = "8086"
	tracer       = otel.Tracer(serviceName)
)

func convertSlogAttrsToAPMAttrsFromSlice(slogAttrs []slog.Attr) []attribute.KeyValue {
    spanAttributes := make([]attribute.KeyValue, 0, len(slogAttrs))
    for _, attr := range slogAttrs {
        spanAttributes = append(spanAttributes, attribute.String(attr.Key, attr.Value.String()))
    }
    return spanAttributes
}

// APMHandler is a custom slog.Handler that adds OpenTelemetry trace/span IDs
// and records errors on the active span.
type APMHandler struct {
	slog.Handler
	attrs []slog.Attr
}

// NewAPMHandler creates a new APMHandler that wraps a base slog.Handler.
func NewAPMHandler(baseHandler slog.Handler) *APMHandler {
	return &APMHandler{
		Handler: baseHandler,
	}
}

// Handle implements slog.Handler.Handle. It adds trace/span IDs and records errors.
func (h *APMHandler) Handle(ctx context.Context, r slog.Record) error {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		spanCtx := span.SpanContext()
		if spanCtx.HasTraceID() {
			r.AddAttrs(slog.String("trace.id", spanCtx.TraceID().String()))
		}
		if spanCtx.HasSpanID() {
			r.AddAttrs(slog.String("span.id", spanCtx.SpanID().String()))
		}

		// Collect ALL attributes: those from logger.With AND those from the current log call
		allAttrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
		allAttrs = append(allAttrs, h.attrs...) // 1. Start with handler's accumulated WithAttrs

		// 2. Add attributes from the current slog.Record itself
		r.Attrs(func(attr slog.Attr) bool {
			allAttrs = append(allAttrs, attr)
			return true
		})		

		// If it's an error level, record the error on the span
		if r.Level == slog.LevelError {
			// Check if an error was explicitly passed as an attribute using slog.Any("error", err)
			var loggedErr error
			r.Attrs(func(attr slog.Attr) bool {
				if attr.Key == "error" { // We'll use "error" as the key for explicit errors
					if errVal, ok := attr.Value.Any().(error); ok {
						loggedErr = errVal
						return false // Found it, stop iterating
					}
				}
				return true // Continue iterating
			})

			if loggedErr != nil {
				span.RecordError(loggedErr, trace.WithAttributes(
					attribute.String("event", "log_error"),
					attribute.String("message", r.Message),
				))
				span.SetStatus(codes.Error, loggedErr.Error())
			} else {
				// Record the log message itself as an error if no specific error object
				span.RecordError(errors.New(r.Message), trace.WithAttributes(
					attribute.String("event", "log_error"),
				))
				span.SetStatus(codes.Error, r.Message)
			}
		} else if r.Level == slog.LevelInfo || r.Level == slog.LevelWarn {
			// Convert slog attributes to OpenTelemetry attributes using the helper function
			APMAttrs := convertSlogAttrsToAPMAttrsFromSlice(allAttrs)
			// Add an event to the OpenTelemetry span with the record's message and converted attributes
			span.AddEvent(r.Message, trace.WithAttributes(APMAttrs...))
		}
	}

	// Always call the base handler to actually output the log record
	return h.Handler.Handle(ctx, r)
}


// WithAttrs implements slog.Handler.WithAttrs.
func (h *APMHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    newAttrs := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
    newAttrs = append(newAttrs, h.attrs...) // Include existing attrs
    newAttrs = append(newAttrs, attrs...)    // Add new attrs

    return &APMHandler{
        Handler: h.Handler.WithAttrs(attrs), // Still pass new attrs to the wrapped handler
        attrs:   newAttrs,                   // Store the combined attrs in the new APMHandler
    }
}

// WithGroup implements slog.Handler.WithGroup.
func (h *APMHandler) WithGroup(name string) slog.Handler {
	return NewAPMHandler(h.Handler.WithGroup(name))
}

// Enabled implements slog.Handler.Enabled.
func (h *APMHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Handler.Enabled(ctx, level)
}



// getTraceID extracts the trace ID from the current span context
func getTraceID(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.HasTraceID() {
		return spanCtx.TraceID().String()
	}
	return "no-trace"
}

// getSpanID extracts the span ID from the current span context
func getSpanID(ctx context.Context) string {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.HasSpanID() {
		return spanCtx.SpanID().String()
	}
	return "no-span"
}

// getTraceSpanInfo extracts both trace and span IDs for logging
func getTraceSpanInfo(ctx context.Context) (string, string) {
	return getTraceID(ctx), getSpanID(ctx)
}

// getEnvOrDefault returns the value of the environment variable or a default value if not set
func getEnvOrDefault(envKey, defaultValue string) string {
	if value := os.Getenv(envKey); value != "" {
		return value
	}
	return defaultValue
}

func initTracerProvider(serviceName string, collectorURL string) (*sdktrace.TracerProvider, error) {
	ctx := context.Background()

	// Buat OTLP HTTP Exporter
	client := otlptracehttp.NewClient(
		otlptracehttp.WithEndpointURL(collectorURL),
	)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat OTLP trace exporter: %w", err)
	}

	// Konfigurasi TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter), // Mengirim trace dalam batch
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
			attribute.String("application", serviceApp),
			attribute.String("environment", serviceEnv),
		)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // Selalu sample semua trace
	)

	// Set TracerProvider global
	otel.SetTracerProvider(tp)

	// Set global propagator untuk header HTTP
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, // W3C Trace Context (standar untuk distributed tracing)
		propagation.Baggage{},
	))

	return tp, nil
}

func main() {
	// 1. Initialize Tracer Provider
	tp, err := initTracerProvider(serviceName, collectorURL)
	if err != nil {
		// Use slog.Error and then os.Exit for fatal errors, as slog doesn't have a direct Fatal
		slog.Default().Error("Gagal menginisialisasi TracerProvider",
			"context", "main",
			slog.Any("error", err), // Use slog.Any for errors
		)
		os.Exit(1)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			slog.Default().Error("Error saat shutdown TracerProvider",
				"context", "main",
				slog.Any("error", err),
			)
		}
	}()

	// 2. Configure slog logger
	// Set up a base JSON handler for structured output to stdout
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true, // Add file and line number to logs
		Level:     slog.LevelDebug,
	})

	// Wrap the base handler with our APMHandler to inject trace/span IDs and handle errors
	logger := slog.New(NewAPMHandler(jsonHandler))

	// Set the default logger for convenience if you want to use slog.Info etc. directly
	slog.SetDefault(logger)

	repo := NewProductRepository()
	service := NewProductService(repo)

	ctx := context.Background()
	http.HandleFunc("/product", func(w http.ResponseWriter, r *http.Request) {
		ctx = otel.GetTextMapPropagator().Extract(r.Context(), 
			propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, "/product",
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.url", r.URL.String()),
			))
		defer span.End()
		// Pass the logger with context. Context not needed if you use AddSource: true
		// handleProduct(ctx, logger.With("context", "handleProduct"), w, r, service)
		handleProduct(ctx, logger, w, r, service)
	})

	port := getEnvOrDefault(EnvPort, DefaultPort)
	addr := ":" + port
	logger.With(
		"context", "main",
		"serviceName", serviceName,
		"address", addr,
	).Info("berjalan")

	if listenErr := http.ListenAndServe(addr, nil); listenErr != nil {
		logger.With(
			"context", "main",
			slog.Any("error", listenErr),
		).Error("Server stopped with an error")
		os.Exit(1)
	}
}

func handleProduct(ctx context.Context, logger *slog.Logger, 
	w http.ResponseWriter, r *http.Request, service ProductService) {
	productID := r.URL.Query().Get("id")
	ctx, span := tracer.Start(ctx, "handleProduct", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()

	if productID == "" {
		logger.With(
			slog.Any("error", "Missing product ID"),
		).ErrorContext(ctx, "Missing product ID")
		http.Error(w, "Parameter 'id' produk diperlukan", http.StatusBadRequest)
		return
	}

	logger.With(
		"productID", productID,
		).DebugContext(ctx, "Mencari info produk")

	// Service Layer: Get Product Info (with trace)
	productInfo, err := service.GetProductInfo(ctx, logger, productID)
	if err != nil {
		logger.With(
			"productID", productID,
			slog.Any("error", err), // Use slog.Any to pass the error object
		).ErrorContext(ctx, "Failed to fetch product info")
		http.Error(w, "Gagal mendapatkan info produk", http.StatusInternalServerError)
		return
	}
	logger.With(
		"productID", productID,
		"productInfo", productInfo,
		).InfoContext(ctx, "Product info fetched")

	logger.With(
		"productID", productID,
		).DebugContext(ctx, "Successfully processed request")
	fmt.Fprint(w, productInfo)
}
