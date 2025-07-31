package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"frontend-service/observability" // <--- IMPORTANT: ensure this is correct
)

var (
	serviceApp   = getEnvOrDefault("APPLICATION", "ecommerce")
	serviceEnv   = getEnvOrDefault("ENVIRONMENT", "development")
	collectorURL = getEnvOrDefault("OTLP_URL", "http://tempo:4318/v1/traces")
	serviceName  = getEnvOrDefault("SERVICE_NAME", "frontend-service")
	EnvPort      = "PORT"
	DefaultPort  = "8085"
)

// getEnvOrDefault returns the value of the environment variable or a default value if not set
func getEnvOrDefault(envKey, defaultValue string) string {
	if value := os.Getenv(envKey); value != "" {
		return value
	}
	return defaultValue
}

func main() {
	// 1. Initialize Tracer Provider via the observability package
	tp, err := observability.SetupTracing(context.Background(), serviceName, serviceApp, serviceEnv, collectorURL)
	if err != nil {
		slog.Default().Error("Failed to initialize TracerProvider via observability package",
			"context", "main",
			slog.Any("error", err),
		)
		os.Exit(1)
	}
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

	productService := NewProductService()
	userService := NewUserService()

	http.HandleFunc("/product-detail", func(w http.ResponseWriter, r *http.Request) {
		// Extract context from incoming request headers for distributed tracing
		ctx := otel.GetTextMapPropagator().Extract(r.Context(),
			propagation.HeaderCarrier(r.Header))

		// Create a new Observability instance for each request
		obs := observability.NewObservability(ctx, baseLogger, serviceName)

		// Start the root span for the HTTP handler using the Observability instance
		ctx, span := obs.Trace.Start(ctx, "/product-detail",
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.url", r.URL.String()),
			))
		defer span.End()

		// Pass the Observability instance down to the handler function
		handleProductDetail(ctx, obs, w, r, productService, userService)
	})

	port := getEnvOrDefault(EnvPort, DefaultPort)
	addr := ":" + port

	// Create a server with explicit timeouts for better security and resilience.
	server := &http.Server{
		Addr:         addr,
		Handler:      nil, // Use DefaultServeMux
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	slog.Info("Server running", "address", addr)

	if listenErr := server.ListenAndServe(); listenErr != nil && listenErr != http.ErrServerClosed {
		slog.Error("Server stopped with an error", "error", listenErr)
		os.Exit(1)
	}
}

// handleProductDetail now centralizes all error handling logic.
func handleProductDetail(ctx context.Context, obs *observability.Observability,
	w http.ResponseWriter, r *http.Request,
	productService ProductService, userService UserService) {

	productID := r.URL.Query().Get("id")

	ctx, span := obs.Trace.Start(ctx, "handleProductDetail", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()

	if productID == "" {
		obs.Log.Error("Missing product ID")
		http.Error(w, "Missing product ID", http.StatusBadRequest)
		return
	}

	obs.Log.Debug("Searching for product info", "productID", productID)

	productInfo, err := productService.GetProductInfo(ctx, obs, productID)
	if err != nil {
		obs.Log.Error("Failed to fetch product info", "error", err)
		http.Error(w, "Failed to fetch product info", http.StatusInternalServerError)
		return
	}

	userID := "user123" // Example user ID
	userInfo, err := userService.GetUserInfo(ctx, obs, userID)
	if err != nil {
		// Not found is a client error, not a server error.
		// The repository already logged a warning, so we just respond.
		obs.Log.Error("Failed to fetch user info", "error", err)
		userInfo = "User info not available"
	}

	obs.Log.Info("Product and user info fetched successfully", "productInfo", productInfo, "userInfo", userInfo)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Detail Produk ID %s:\n%s\nInfo Pengguna:\n%s", productID, productInfo, userInfo)
}
