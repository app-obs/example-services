package main

import (
	"context"
	"log"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ProductService interface {
	GetProductInfo(ctx context.Context, productID string) (string, error)
}

type productServiceImpl struct {
	repo ProductRepository
}

func (s *productServiceImpl) GetProductInfo(ctx context.Context, productID string) (string, error) {
	ctx, span := tracer.Start(ctx, "ProductService.GetProductInfo", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()

	traceID, spanID := getTraceSpanInfo(ctx)
	log.Printf("[%s|%s] ProductService: Processing request for product ID %s", traceID, spanID, productID)

	productInfo, err := s.repo.FetchProduct(ctx, productID)
	if err != nil {
		log.Printf("[%s|%s] ProductService: Error fetching product %s: %v", traceID, spanID, productID, err)
		return "", err
	}

	log.Printf("[%s|%s] ProductService: Successfully retrieved product info for %s", traceID, spanID, productID)
	return productInfo, nil
}

func NewProductService(repo ProductRepository) ProductService {
	return &productServiceImpl{repo: repo}
}
