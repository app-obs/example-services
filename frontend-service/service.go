package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"frontend-service/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var (
	productServiceURL = getEnvOrDefault("PRODUCT_SERVICE_URL", "http://product-service:8086")
	userServiceURL    = getEnvOrDefault("USER_SERVICE_URL", "http://user-service:8087")
)

type ProductService interface {
	GetProductInfo(ctx context.Context, obs *observability.Observability, productID string) (string, error)
}

type UserService interface {
	GetUserInfo(ctx context.Context, obs *observability.Observability, userID string) (string, error)
}

// Implementation for calling external services

type productServiceImpl struct{}

func (s *productServiceImpl) GetProductInfo(ctx context.Context, obs *observability.Observability, productID string) (string, error) {
	ctx, span := obs.Trace.Start(ctx, "ProductService.GetProductInfo", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()
	return callProductService(ctx, obs, productID)
}

type userServiceImpl struct{}

func (s *userServiceImpl) GetUserInfo(ctx context.Context, obs *observability.Observability, userID string) (string, error) {
	ctx, span := obs.Trace.Start(ctx, "UserService.GetUserInfo", trace.WithAttributes(attribute.String("user.id", userID)))
	defer span.End()
	return callUserService(ctx, obs, userID)
}

func NewProductService() ProductService {
	return &productServiceImpl{}
}

func NewUserService() UserService {
	return &userServiceImpl{}
}

func callProductService(ctx context.Context, obs *observability.Observability, productID string) (string, error) {

	ctx, span := obs.Trace.Start(ctx, "callProductService", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/product?id=%s", productServiceURL, productID), nil)
	if err != nil {
		return "", err
	}

	// Inject konteks trace ke dalam header HTTP
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("product service returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func callUserService(ctx context.Context, obs *observability.Observability, userID string) (string, error) {

	ctx, span := obs.Trace.Start(ctx, "callUserService", trace.WithAttributes(attribute.String("user.id", userID)))
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/user?id=%s", userServiceURL, userID), nil)
	if err != nil {
		return "", err
	}

	// Inject konteks trace ke dalam header HTTP
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("user service returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}