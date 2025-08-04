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
	DefaultPort = "8087"
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

	repo := NewUserRepository()
	service := NewUserService(repo)

	http.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		r, ctx, span, _ := obsFactory.StartSpanFromRequest(r)
		defer span.End()
		handleUser(ctx, w, r, service)
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

// handleUser now centralizes all error handling logic.
func handleUser(ctx context.Context,
	w http.ResponseWriter, r *http.Request, service UserService) {
	userID := r.URL.Query().Get("id")

	obs := observability.ObsFromCtx(ctx)
	ctx, span := obs.StartSpan(ctx, "handleUser", observability.SpanAttributes{"user.id": userID})
	defer span.End()

	if userID == "" {
		obs.ErrorHandler.HTTP(w, "Missing user ID", http.StatusBadRequest)
		return
	}

	obs.Log.Debug("Searching for user info", "userID", userID)

	userInfo, err := service.GetUserInfo(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			obs.ErrorHandler.HTTP(w, "User not found", http.StatusNotFound)
		} else {
			obs.ErrorHandler.HTTP(w, "Failed to fetch user info", http.StatusInternalServerError)
		}
		return
	}

	obs.Log.Info("User info fetched successfully", "userInfo", userInfo)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(userInfo))
}