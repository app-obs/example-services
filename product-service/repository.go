package main

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ProductRepository interface {
	FetchProduct(ctx context.Context, logger *slog.Logger, productID string) (string, error)
}

type productRepositoryImpl struct{}

func (r *productRepositoryImpl) FetchProduct(ctx context.Context, logger *slog.Logger, 
	productID string) (string, error) {
	
	ctx, span := tracer.Start(ctx, "ProductRepository.FetchProduct", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()

	logger.With(
		"productID", productID,
	).DebugContext(ctx, "Fetching product data")

	// Simulate DB fetch
	productInfo := "Nama: Produk " + productID + ", Harga: $19.99, Stok: 100"

	logger.With(
		"productID", productID,
		"productInfo", productInfo,
	).InfoContext(ctx, "Successfully fetched product data")
	return productInfo, nil
}

func NewProductRepository() ProductRepository {
	return &productRepositoryImpl{}
}
