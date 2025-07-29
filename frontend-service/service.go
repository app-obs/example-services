package main

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	ctx, span := tracer.Start(ctx, "ProductService.GetProductInfo", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()
	return callProductService(ctx, productID)
}

type userServiceImpl struct{}

func (s *userServiceImpl) GetUserInfo(ctx context.Context, userID string) (string, error) {
	ctx, span := tracer.Start(ctx, "UserService.GetUserInfo", trace.WithAttributes(attribute.String("user.id", userID)))
	defer span.End()
	return callUserService(ctx, userID)
}

func NewProductService() ProductService {
	return &productServiceImpl{}
}

func NewUserService() UserService {
	return &userServiceImpl{}
}
