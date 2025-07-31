package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"product-service/observability" // <--- IMPORTANT: ensure this is correct
)

var (
	serviceApp    = getEnvOrDefault("APPLICATION", "ecommerce")
	serviceEnv    = getEnvOrDefault("ENVIRONMENT", "development")
	collectorURL  = getEnvOrDefault("OTLP_URL", "http://tempo:4318/v1/traces")
	serviceName   = getEnvOrDefault("SERVICE_NAME", "product-service")
	EnvPort       = "PORT"
	DefaultPort   = "8086"
)

// getEnvOrDefault returns the value of the environment variable or a default value if not set
func getEnvOrDefault(envKey, defaultValue string) string {
	if value := os.Getenv(envKey); value != "" {
		return value
	}
	return defaultValue
}

// Removed initTracerProvider - its logic is now in observability.SetupTracing

func main() {
	// 1. Initialize Tracer Provider via the observability package
	// Changed: Call observability.SetupTracing
	tp, err := observability.SetupTracing(context.Background(), serviceName, serviceApp, serviceEnv, collectorURL)
	if err != nil {
		slog.Default().Error("Failed to initialize TracerProvider via observability package",
			"context", "main",
			slog.Any("error", err),
		)
		os.Exit(1)
	}
	// The defer for shutdown remains here as main is responsible for the overall app lifecycle
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			slog.Default().Error("Error shutting down TracerProvider",
				"context", "main",
				slog.Any("error", err),
			)
		}
	}()

	// 2. Configure slog logger
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})

	// Wrap the base handler with our APMHandler to inject trace/span IDs and handle errors
	baseLogger := slog.New(observability.NewAPMHandler(jsonHandler))
	slog.SetDefault(baseLogger) // Set the default slog logger for general use

	repo := NewProductRepository()
	service := NewProductService(repo)

	http.HandleFunc("/product", func(w http.ResponseWriter, r *http.Request) {
		// Extract context from incoming request headers for distributed tracing
		ctx := otel.GetTextMapPropagator().Extract(r.Context(),
			propagation.HeaderCarrier(r.Header))

		// Create a new Observability instance for each request
		obs := observability.NewObservability(ctx, baseLogger, serviceName)

		// Start the root span for the HTTP handler using the Observability instance
		ctx, span := obs.Trace.Start(ctx, "/product",
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.url", r.URL.String()),
			))
		defer span.End()

		// Pass the Observability instance down to the handler function
		handleProduct(ctx, obs, w, r, service)
	})

	port := getEnvOrDefault(EnvPort, DefaultPort)
	addr := ":" + port
	slog.With( // Using slog.Default() here, which is set to baseLogger
		"context", "main",
		"serviceName", serviceName,
		"address", addr,
	).Info("Server running")

	if listenErr := http.ListenAndServe(addr, nil); listenErr != nil {
		slog.With( // Using slog.Default() here
			"context", "main",
			slog.Any("error", listenErr),
		).Error("Server stopped with an error")
		os.Exit(1)
	}
}

// handleProduct now receives the Observability struct
func handleProduct(ctx context.Context, obs *observability.Observability,
	w http.ResponseWriter, r *http.Request, service ProductService) {

	productID := r.URL.Query().Get("id")

	// Start a new span using obs.Trace
	ctx, span := obs.Trace.Start(ctx, "handleProduct", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()

	if productID == "" {
		// Log using obs.Log, context is implicitly handled
		obs.Log.With(
			slog.Any("error", "Missing product ID"),
		).Error("Missing product ID")
		http.Error(w, "Parameter 'id' produk diperlukan", http.StatusBadRequest)
		return
	}

	obs.Log.With(
		"productID", productID,
	).Debug("Searching for product info")

	// Service Layer: Get Product Info (with trace)
	// Pass the Observability struct down to the service layer
	productInfo, err := service.GetProductInfo(ctx, obs, productID)
	if err != nil {
		obs.Log.With(
			"productID", productID,
			slog.Any("error", err),
		).Error("Failed to fetch product info")
		http.Error(w, "Gagal mendapatkan info produk", http.StatusInternalServerError)
		return
	}
	obs.Log.With(
		"productID", productID,
		"productInfo", productInfo,
	).Info("Product info fetched")

	obs.Log.With(
		"productID", productID,
	).Debug("Successfully processed request")
	fmt.Fprint(w, productInfo)
}

