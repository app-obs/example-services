package main

import (
	"context"
	"product-service/observability"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ProductRepository interface {
	GetProductByID(ctx context.Context, obs *observability.Observability, id string) (string, error)
}

type productRepositoryImpl struct{}

func (r *productRepositoryImpl) GetProductByID(ctx context.Context, obs *observability.Observability, id string) (string, error) {

	ctx, span := obs.Trace.Start(ctx, "ProductRepository.GetProductByID", trace.WithAttributes(attribute.String("product.id", id)))
	defer span.End()

	obs.Log.With(
		"productID", id,
	).Debug("Fetching product data")

	// Simulate DB fetch
	// if id == "123" {
	// 	obs.Log.With("productID", id).Debug("Product found in repository")
	// 	return "Product ABC", nil
	// }
	// obs.Log.With("productID", id).Warn("Product not found in repository")
	// return "", fmt.Errorf("product not found: %s", id)
	obs.Log.With("productID", id).Debug("Product found in repository")
	return "Product ABC", nil
}

func NewProductRepository() ProductRepository {
	return &productRepositoryImpl{}
}