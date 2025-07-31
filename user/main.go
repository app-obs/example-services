package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"user-service/observability" // <--- IMPORTANT: ensure this is correct
)

var (
	serviceApp  = getEnvOrDefault("APPLICATION", "ecommerce")
	serviceEnv  = getEnvOrDefault("ENVIRONMENT", "development")
	APMURL      = getEnvOrDefault("APM_URL", "http://tempo:4318/v1/traces")
	serviceName = getEnvOrDefault("SERVICE_NAME", "user-service")
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
	// Create a background Observability instance for application-level logging
	bgObs := observability.NewObservability(context.Background(), serviceName)

	// 1. Initialize Tracer Provider via the observability package
	tp, err := observability.SetupTracing(context.Background(), serviceName, serviceApp, serviceEnv, APMURL)
	if err != nil {
		bgObs.Log.Error("Failed to initialize TracerProvider", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			bgObs.Log.Error("Error shutting down TracerProvider", "error", err)
		}
	}()

	repo := NewUserRepository()
	service := NewUserService(repo)

	http.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		// Create a new Observability instance from the request
		obs := observability.NewObservabilityFromRequest(r, serviceName)

		// Start the root span for the HTTP handler using the Observability instance
		ctx, span := obs.Trace.Start(obs.Context(), "/user",
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.url", r.URL.String()),
			))
		defer span.End()

		// Pass the Observability instance down to the handler function
		ctx = CtxWithObs(ctx, obs)
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
		bgObs.Log.Error("Server stopped with an error", "error", listenErr)
		os.Exit(1)
	}
}

// handleUser now centralizes all error handling logic.
func handleUser(ctx context.Context,
	w http.ResponseWriter, r *http.Request, service UserService) {
	obs := ObsFromCtx(ctx)
	userID := r.URL.Query().Get("id")

	ctx, span := obs.Trace.Start(ctx, "handleUser", trace.WithAttributes(attribute.String("user.id", userID)))
	defer span.End()

	if userID == "" {
		obs.Log.Error("Missing user ID")
		http.Error(w, "Missing user ID", http.StatusBadRequest)
		return
	}

	obs.Log.Debug("Searching for user info", "userID", userID)

	userInfo, err := service.GetUserInfo(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Not found is a client error, not a server error.
			// The repository already logged a warning, so we just respond.
			http.Error(w, "User not found", http.StatusNotFound)
		} else {
			// For all other errors, log them as server errors and respond.
			obs.Log.Error("Failed to fetch user info", "error", err)
			http.Error(w, "Failed to fetch user info", http.StatusInternalServerError)
		}
		return
	}

	obs.Log.Info("User info fetched successfully", "userInfo", userInfo)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(userInfo))
}
