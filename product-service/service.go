package main

import (
	"context"
	"product-service/observability"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
)

type ProductService interface {
	GetProductInfo(ctx context.Context, obs *observability.Observability, productID string) (string, error)
}

type productServiceImpl struct {
	repo ProductRepository
}

func (s *productServiceImpl) GetProductInfo(ctx context.Context, obs *observability.Observability, productID string) (string, error) {

	ctx, span := obs.Trace.Start(ctx, "ProductService.GetProductInfo", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()

	obs.Log.With(
		"productID", productID,
	).Debug("Processing request")

	productInfo, err := s.repo.GetProductByID(ctx, obs, productID)
	if err != nil {
		obs.Log.With(
			"productID", productID,
			slog.Any("error", err),
		).Error("Error fetching product")
		return "", err
	}

	obs.Log.With(
		"productID", productID,
		"productInfo", productInfo,
	).Info("Successfully retrieved product info")
	return productInfo, nil
}

func NewProductService(repo ProductRepository) ProductService {
	return &productServiceImpl{repo: repo}
}