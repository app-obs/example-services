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
