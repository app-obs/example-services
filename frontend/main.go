package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/app-obs/go/observability"
)

var (
	EnvPort     = "PORT"
	DefaultPort = "8085"
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

	// 1. Initialize all observability components
	shutdown, err := obsFactory.Setup(context.Background())
	if err != nil {
		// If setup fails, we can't rely on the full observability stack.
		// Log to a fallback logger (e.g., standard library logger) and exit.
		slog.Error("Failed to setup observability", "error", err)
		os.Exit(1)
	}

	// Now that setup is complete, create the background observability instance.
	bgObs := obsFactory.NewBackgroundObservability(context.Background())

	defer func() {
		if err := shutdown.Shutdown(context.Background()); err != nil {
			bgObs.Log.Error("Error during observability shutdown", "error", err)
		}
	}()

	// The services rely on the following environment variables to connect to backends:
	// - PRODUCT_SERVICE_URL: The URL for the product service.
	// - USER_SERVICE_URL: The URL for the user service.
	productService := NewProductService()
	userService := NewUserService()

	http.HandleFunc("/product-detail", func(w http.ResponseWriter, r *http.Request) {
		r, ctx, span, _ := obsFactory.StartSpanFromRequest(r)
		defer span.End()
		handleProductDetail(ctx, w, r, productService, userService)
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
		bgObs.ErrorHandler.Fatal("Server stopped with an error", "error", listenErr)
	}
}

// handleProductDetail now centralizes all error handling logic.
func handleProductDetail(ctx context.Context,
	w http.ResponseWriter, r *http.Request,
	productService ProductService, userService UserService) {
	productID := r.URL.Query().Get("id")

	obs := observability.ObsFromCtx(ctx)
	ctx, span := obs.StartSpan(ctx, "handleProductDetail", observability.SpanAttributes{"product.id": productID})
	defer span.End()

	if productID == "" {
		obs.ErrorHandler.HTTP(w, "Missing product ID", http.StatusBadRequest)
		return
	}

	obs.Log.Debug("Searching for product info", "productID", productID)

	productInfo, err := productService.GetProductInfo(ctx, productID)
	if err != nil {
		obs.ErrorHandler.HTTP(w, "Failed to fetch product info", http.StatusInternalServerError)
		return
	}

	userID := "user123" // Example user ID
	userInfo, err := userService.GetUserInfo(ctx, userID)
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