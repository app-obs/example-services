package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/app-obs/go/observability"
)

var (
	productServiceURL = getEnvOrDefault("PRODUCT_SERVICE_URL", "http://product-service:8086")
	userServiceURL    = getEnvOrDefault("USER_SERVICE_URL", "http://user-service:8087")
)

type ProductService interface {
	GetProductInfo(ctx context.Context, productID string) (string, error)
}

type UserService interface {
	GetUserInfo(ctx context.Context, userID string) (string, error)
}

// Implementation for calling external services

type productServiceImpl struct{}

func (s *productServiceImpl) GetProductInfo(ctx context.Context, productID string) (string, error) {
	ctx, _, span := observability.StartSpanFromCtx(ctx, "ProductService.GetProductInfo", observability.SpanAttributes{"product.id": productID})
	defer span.End()
	return callProductService(ctx, productID)
}

type userServiceImpl struct{}

func (s *userServiceImpl) GetUserInfo(ctx context.Context, userID string) (string, error) {
	ctx, _, span := observability.StartSpanFromCtx(ctx, "UserService.GetUserInfo", observability.SpanAttributes{"user.id": userID})
	defer span.End()
	return callUserService(ctx, userID)
}

func NewProductService() ProductService {
	return &productServiceImpl{}
}

func NewUserService() UserService {
	return &userServiceImpl{}
}

func callProductService(ctx context.Context, productID string) (string, error) {
	ctx, obs, span := observability.StartSpanFromCtx(ctx, "callProductService", observability.SpanAttributes{"product.id": productID})
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/product?id=%s", productServiceURL, productID), nil)
	if err != nil {
		return "", err
	}
	obs.Trace.InjectHTTP(req)

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

func callUserService(ctx context.Context, userID string) (string, error) {
	ctx, obs, span := observability.StartSpanFromCtx(ctx, "callUserService", observability.SpanAttributes{"user.id": userID})
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/user?id=%s", userServiceURL, userID), nil)
	if err != nil {
		return "", err
	}
	obs.Trace.InjectHTTP(req)

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