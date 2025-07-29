package main

import (
	"context"
	"fmt"
	"log"
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
	serviceName      = getEnvOrDefault("SERVICE_NAME", "product-service")
	collectorURL     = getEnvOrDefault("OPENOBSERVE_URL", "http://36.64.55.158:5080")
	EnvOtlpAuthToken = "OPENOBSERVE_AUTH_TOKEN"
	DefaultAuthToken = ""
	EnvPort          = "PORT"
	DefaultPort      = "8086"
	tracer           = otel.Tracer(serviceName)
)

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
		otlptracehttp.WithEndpoint(collectorURL),
		otlptracehttp.WithInsecure(),                        // Gunakan ini jika OpenObserve tidak menggunakan HTTPS
		otlptracehttp.WithURLPath("/api/default/v1/traces"), // Path untuk OpenObserve
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Basic " + getEnvOrDefault(EnvOtlpAuthToken, DefaultAuthToken),
		}),
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
			attribute.String("application", "ecommerce"),
			attribute.String("environment", "development"),
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
	tp, err := initTracerProvider(serviceName, collectorURL)
	if err != nil {
		log.Fatalf("[system] Gagal menginisialisasi TracerProvider: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("[system] Error saat shutdown TracerProvider: %v", err)
		}
	}()

	repo := NewProductRepository()
	service := NewProductService(repo)

	http.HandleFunc("/product", func(w http.ResponseWriter, r *http.Request) {
		handleProduct(w, r, service)
	})

	port := getEnvOrDefault(EnvPort, DefaultPort)
	addr := ":" + port
	log.Printf("[%s] %s berjalan di %s", "system", serviceName, addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handleProduct(w http.ResponseWriter, r *http.Request, service ProductService) {
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	ctx, span := tracer.Start(ctx, "handleProduct",
		trace.WithAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.url", r.URL.String()),
		))
	defer span.End()

	productID := r.URL.Query().Get("id")
	if productID == "" {
		http.Error(w, "Parameter 'id' produk diperlukan", http.StatusBadRequest)
		span.SetStatus(codes.Error, "Missing product ID")
		return
	}
	span.SetAttributes(attribute.String("product.id", productID))

	traceID, spanID := getTraceSpanInfo(ctx)
	log.Printf("[%s|%s] Product Service: Mencari info produk ID %s", traceID, spanID, productID)

	// Service Layer: Get Product Info (with trace)
	productInfo, err := service.GetProductInfo(ctx, productID)
	if err != nil {
		log.Printf("[%s|%s] Product Service: Error processing product ID %s: %v", traceID, spanID, productID, err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to fetch product info")
		http.Error(w, "Gagal mendapatkan info produk", http.StatusInternalServerError)
		return
	}
	span.AddEvent("Product data fetched", trace.WithAttributes(attribute.String("product.info", productInfo)))

	log.Printf("[%s|%s] Product Service: Successfully processed request for product ID %s", traceID, spanID, productID)
	fmt.Fprint(w, productInfo)
}
