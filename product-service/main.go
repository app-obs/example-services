package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
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

// handleProduct now centralizes all error handling logic.
func handleProduct(ctx context.Context, obs *observability.Observability,
	w http.ResponseWriter, r *http.Request, service ProductService) {

	productID := r.URL.Query().Get("id")

	ctx, span := obs.Trace.Start(ctx, "handleProduct", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()

	if productID == "" {
		obs.Log.Error("Missing product ID")
		http.Error(w, "Missing product ID", http.StatusBadRequest)
		return
	}

	obs.Log.Debug("Searching for product info", "productID", productID)

	productInfo, err := service.GetProductInfo(ctx, obs, productID)
	if err != nil {
		if errors.Is(err, ErrProductNotFound) {
			// Not found is a client error, not a server error.
			// The repository already logged a warning, so we just respond.
			http.Error(w, "Product not found", http.StatusNotFound)
		} else {
			// For all other errors, log them as server errors and respond.
			obs.Log.Error("Failed to fetch product info", "error", err)
			http.Error(w, "Failed to fetch product info", http.StatusInternalServerError)
		}
		return
	}

	obs.Log.Info("Product info fetched successfully", "productInfo", productInfo)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(productInfo))
}