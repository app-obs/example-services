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

func main() {
	// The factory will automatically read the following environment variables:
	// - OBS_SERVICE_NAME: The name of the service.
	// - OBS_APPLICATION: The name of the application.
	// - OBS_ENVIRONMENT: The deployment environment (e.g., "development", "production").
	// - OBS_APM_TYPE: The APM backend to use ("otlp", "datadog", or "none").
	// - OBS_APM_URL: The URL of the APM collector.
	obsFactory := observability.NewFactory()

	// 1. Initialize all observability components, exiting on failure.
	shutdowner := obsFactory.SetupOrExit("Failed to setup observability")

	// Now that setup is complete, create the background observability instance.
	bgObs := obsFactory.NewBackgroundObservability(context.Background())

	// 2. Defer the shutdown call.
	defer shutdowner.ShutdownOrLog("Error during observability shutdown")

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