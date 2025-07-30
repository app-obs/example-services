package main

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ProductService interface {
	GetProductInfo(ctx context.Context, logger *slog.Logger, productID string) (string, error)
}

type productServiceImpl struct {
	repo ProductRepository
}

func (s *productServiceImpl) GetProductInfo(ctx context.Context, logger *slog.Logger, 
	productID string) (string, error) {

	ctx, span := tracer.Start(ctx, "ProductService.GetProductInfo", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()

	logger.With(
		"productID", productID,
	).DebugContext(ctx, "Processing request")

	productInfo, err := s.repo.FetchProduct(ctx, logger, productID)
	if err != nil {
		logger.With(
			"productID", productID,
		).ErrorContext(ctx, "Error fetching product")
		return "", err
	}

	logger.With(
		"productID", productID,
		"productInfo", productInfo,
	).InfoContext(ctx, "Successfully retrieved product info")
	return productInfo, nil
}

func NewProductService(repo ProductRepository) ProductService {
	return &productServiceImpl{repo: repo}
}
