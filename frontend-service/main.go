package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation" // Untuk otel.SetTextMapPropagator
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	serviceApp  = getEnvOrDefault("APPLICATION", "ecommerce")
	serviceEnv  = getEnvOrDefault("ENVIRONMENT", "development")
	collectorURL = getEnvOrDefault("OTLP_URL", "tempo:4318")
	collectorHTTPPath = getEnvOrDefault("OTLP_HTTP_PATH", "/v1/traces")

	serviceName       = getEnvOrDefault("SERVICE_NAME", "frontend-service")
	productServiceURL = getEnvOrDefault("PRODUCT_SERVICE_URL", "http://product-service:8086")
	userServiceURL    = getEnvOrDefault("USER_SERVICE_URL", "http://user-service:8087")
	EnvPort           = "PORT"
	DefaultPort       = "8085"
	tracer            = otel.Tracer(serviceName)
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
		otlptracehttp.WithInsecure(),            // Gunakan ini jika Tempo OTLP
		otlptracehttp.WithURLPath(collectorHTTPPath), // Path untuk Tempo OTLP
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
		log.Fatalf("Gagal menginisialisasi TracerProvider: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("[system] Error saat shutdown TracerProvider: %v", err)
		}
	}()

	// productRepo := NewProductRepository()
	// userRepo := NewUserRepository()
	productService := NewProductService()
	userService := NewUserService()

	http.HandleFunc("/product-detail", func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		// fmt.Println(propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, "/product-detail",
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.url", r.URL.String()),
			))
		defer span.End()

		handleProductDetail(ctx, w, r, productService, userService)
	})

	port := getEnvOrDefault(EnvPort, DefaultPort)
	addr := ":" + port
	log.Printf("[system] %s berjalan di %s", serviceName, addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handleProductDetail(ctx context.Context, w http.ResponseWriter, r *http.Request, productService ProductService, userService UserService) {
	// ctx := r.Context()
	ctx, span := tracer.Start(ctx, "handleProductDetail")
	defer span.End()

	productID := r.URL.Query().Get("id")
	if productID == "" {
		http.Error(w, "Parameter 'id' produk diperlukan", http.StatusBadRequest)
		span.SetStatus(codes.Error, "Missing product ID")
		return
	}
	span.SetAttributes(attribute.String("product.id", productID))

	traceID, spanID := getTraceSpanInfo(ctx)
	log.Printf("[%s|%s] Frontend Service: Menerima permintaan untuk produk ID %s", traceID, spanID, productID)

	// Service Layer: Get Product Info (with trace)
	productInfo, err := productService.GetProductInfo(ctx, productID)
	if err != nil {
		log.Printf("[%s|%s] Error memanggil Product Service: %v", traceID, spanID, err)
		http.Error(w, "Gagal mendapatkan info produk", http.StatusInternalServerError)
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to call Product Service")
		return
	}
	span.AddEvent("Received product info", trace.WithAttributes(attribute.String("product.info", productInfo)))

	// Service Layer: Get User Info (with trace)
	userID := "user123" // Contoh user ID
	userInfo, err := userService.GetUserInfo(ctx, userID)
	if err != nil {
		log.Printf("[%s|%s] Error memanggil User Service: %v", traceID, spanID, err)
		span.RecordError(err)
		span.AddEvent("Failed to get user info", trace.WithAttributes(attribute.String("user.id", userID)))
		userInfo = "User info not available"
	}
	span.AddEvent("Received user info", trace.WithAttributes(attribute.String("user.info", userInfo)))

	log.Printf("[%s|%s] Frontend Service: Successfully processed request for product ID %s", traceID, spanID, productID)
	fmt.Fprintf(w, "Detail Produk ID %s:\n%s\nInfo Pengguna:\n%s", productID, productInfo, userInfo)
}

func callProductService(ctx context.Context, productID string) (string, error) {

	ctx, span := tracer.Start(ctx, "callProductService", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/product?id=%s", productServiceURL, productID), nil)
	if err != nil {
		return "", err
	}

	// Inject konteks trace ke dalam header HTTP
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("product service returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func callUserService(ctx context.Context, userID string) (string, error) {

	ctx, span := tracer.Start(ctx, "callUserService", trace.WithAttributes(attribute.String("user.id", userID)))
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/user?id=%s", userServiceURL, userID), nil)
	if err != nil {
		return "", err
	}

	// Inject konteks trace ke dalam header HTTP
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("user service returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
