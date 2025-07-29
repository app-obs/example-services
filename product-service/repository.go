package main

import (
	"context"
	"log"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ProductRepository interface {
	FetchProduct(ctx context.Context, productID string) (string, error)
}

type productRepositoryImpl struct{}

func (r *productRepositoryImpl) FetchProduct(ctx context.Context, productID string) (string, error) {
	ctx, span := tracer.Start(ctx, "ProductRepository.FetchProduct", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()

	traceID, spanID := getTraceSpanInfo(ctx)
	log.Printf("[%s|%s] ProductRepository: Fetching product data for ID %s", traceID, spanID, productID)

	// Simulate DB fetch
	productInfo := "Nama: Produk " + productID + ", Harga: $19.99, Stok: 100"

	log.Printf("[%s|%s] ProductRepository: Successfully fetched product data for ID %s", traceID, spanID, productID)
	return productInfo, nil
}

func NewProductRepository() ProductRepository {
	return &productRepositoryImpl{}
}
