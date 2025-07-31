package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"product/observability" // <--- IMPORTANT: ensure this is correct
)

var (
	serviceApp  = getEnvOrDefault("APPLICATION", "ecommerce")
	serviceEnv  = getEnvOrDefault("ENVIRONMENT", "development")
	APMType     = observability.APMType(getEnvOrDefault("APM_TYPE", "OTLP"))
	APMURL      = getEnvOrDefault("APM_URL", "http://tempo:4318/v1/traces")
	serviceName = getEnvOrDefault("SERVICE_NAME", "product-service")
	EnvPort     = "PORT"
	DefaultPort = "8086"
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
	// Create a background Observability instance for application-level logging
	bgObs := observability.NewObservability(context.Background(), serviceName, APMType)

	// 1. Initialize Tracer Provider via the observability package
	// Changed: Call observability.SetupTracing
	tp, err := observability.SetupTracing(context.Background(), serviceName, serviceApp, serviceEnv, APMURL, string(APMType))
	if err != nil {
		bgObs.Log.Error("Failed to initialize TracerProvider", "error", err)
		os.Exit(1)
	}
	// The defer for shutdown remains here as main is responsible for the overall app lifecycle
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			// Use a background obs instance to log shutdown errors
			bgObs.Log.Error("Error shutting down TracerProvider", "error", err)
		}
	}()

	repo := NewProductRepository()
	service := NewProductService(repo)

	http.HandleFunc("/product", func(w http.ResponseWriter, r *http.Request) {
		// Create a new Observability instance from the request
		obs := observability.NewObservabilityFromRequest(r, serviceName, APMType)

		// Start the root span for the HTTP handler using the Observability instance
		ctx, span := obs.Trace.Start(obs.Context(), "/product")
		span.SetAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.url", r.URL.String()),
		)
		defer span.End()
		// Pass the Observability instance down to the handler function
		ctx = observability.CtxWithObs(ctx, obs)
		handleProduct(ctx, w, r, service)
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

	bgObs.Log.Info("Server running", "address", addr)

	if listenErr := server.ListenAndServe(); listenErr != nil && listenErr != http.ErrServerClosed {
		bgObs.Log.Error("Server stopped with an error", "error", listenErr)
		os.Exit(1)
	}
}

// handleProduct now centralizes all error handling logic.
func handleProduct(ctx context.Context,
	w http.ResponseWriter, r *http.Request, service ProductService) {
	obs := observability.ObsFromCtx(ctx)
	productID := r.URL.Query().Get("id")

	ctx, span := obs.Trace.Start(ctx, "handleProduct")
	span.SetAttributes(attribute.String("product.id", productID))
	defer span.End()

	if productID == "" {
		obs.Log.Error("Missing product ID")
		http.Error(w, "Missing product ID", http.StatusBadRequest)
		return
	}

	obs.Log.Debug("Searching for product info", "productID", productID)

	productInfo, err := service.GetProductInfo(ctx, productID)
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
