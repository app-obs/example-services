package main

import (
	"context"
	"product/observability"

	"go.opentelemetry.io/otel/attribute"
)

type ProductService interface {
	GetProductInfo(ctx context.Context, productID string) (string, error)
}

type productServiceImpl struct {
	repo ProductRepository
}

func (s *productServiceImpl) GetProductInfo(ctx context.Context, productID string) (string, error) {
	obs := observability.ObsFromCtx(ctx)
	ctx, span := obs.Trace.Start(ctx, "ProductService.GetProductInfo")
	span.SetAttributes(attribute.String("product.id", productID))
	defer span.End()

	obs.Log.With(
		"productID", productID,
	).Debug("Processing request")

	productInfo, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		obs.Log.With(
			"productID", productID,
			"error", err,
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
