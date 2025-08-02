package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/app-obs/go/observability"
)

var (
	serviceApp  = getEnvOrDefault("APPLICATION", "ecommerce")
	serviceEnv  = getEnvOrDefault("ENVIRONMENT", "development")
	APMType     = getEnvOrDefault("APM_TYPE", "none")
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
	factoryConfig := observability.FactoryConfig{
		ServiceName: serviceName,
		ServiceApp:  serviceApp,
		ServiceEnv:  serviceEnv,
		ApmType:     APMType,
		ApmURL:      APMURL,
	}
	obsFactory := observability.NewFactory(factoryConfig)
	bgObs := obsFactory.NewBackgroundObservability(context.Background())

	// 1. Initialize Tracer Provider via the factory
	tp, err := obsFactory.SetupTracing(context.Background())
	if err != nil {
		bgObs.ErrorHandler.Fatal("Failed to initialize TracerProvider", "error", err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			bgObs.Log.Error("Error shutting down TracerProvider", "error", err)
		}
	}()

	repo := NewProductRepository()
	service := NewProductService(repo)

	http.HandleFunc("/product", func(w http.ResponseWriter, r *http.Request) {
		r, ctx, span, _ := obsFactory.StartSpanFromRequest(r)
		defer span.End()
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
	productID := r.URL.Query().Get("id")

	obs := observability.ObsFromCtx(ctx)
	ctx, span := obs.StartSpan(ctx, "handleProduct", observability.SpanAttributes{"product.id": productID})
	defer span.End()

	if productID == "" {
		obs.ErrorHandler.HTTP(w, "Missing product ID", http.StatusBadRequest)
		return
	}

	obs.Log.Debug("Searching for product info", "productID", productID)

	productInfo, err := service.GetProductInfo(ctx, productID)
	if err != nil {
		if errors.Is(err, ErrProductNotFound) {
			obs.ErrorHandler.HTTP(w, "Product not found", http.StatusNotFound)
		} else {
			obs.ErrorHandler.HTTP(w, "Failed to fetch product info", http.StatusInternalServerError)
		}
		return
	}

	obs.Log.Info("Product info fetched successfully", "productInfo", productInfo)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(productInfo))
}
