package main

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ProductRepository interface {
	FetchProduct(ctx context.Context, productID string) (string, error)
}

type UserRepository interface {
	FetchUser(ctx context.Context, userID string) (string, error)
}

// Dummy repository implementation (simulate DB)
type productRepositoryImpl struct{}

type userRepositoryImpl struct{}

func (r *productRepositoryImpl) FetchProduct(ctx context.Context, productID string) (string, error) {
	_, span := tracer.Start(ctx, "ProductRepository.FetchProduct", trace.WithAttributes(attribute.String("product.id", productID)))
	defer span.End()
	// Simulate DB fetch
	return "Nama: Produk " + productID + ", Harga: $19.99, Stok: 100", nil
}

func (r *userRepositoryImpl) FetchUser(ctx context.Context, userID string) (string, error) {
	_, span := tracer.Start(ctx, "UserRepository.FetchUser", trace.WithAttributes(attribute.String("user.id", userID)))
	defer span.End()
	return "Nama: User " + userID + ", Email: user" + userID + "@example.com", nil
}

func NewProductRepository() ProductRepository {
	return &productRepositoryImpl{}
}

func NewUserRepository() UserRepository {
	return &userRepositoryImpl{}
}
