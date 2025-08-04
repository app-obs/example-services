package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/app-obs/go/observability"
)

// ErrProductNotFound is returned when a product is not found.
var ErrProductNotFound = errors.New("product not found")

type ProductRepository interface {
	GetProductByID(ctx context.Context, id string) (string, error)
}

type productRepositoryImpl struct{}

func (r *productRepositoryImpl) GetProductByID(ctx context.Context, id string) (string, error) {
	ctx, obs, span := observability.StartSpanFromCtx(ctx, "ProductRepository.GetProductByID", observability.SpanAttributes{"product.id": id})
	defer span.End()

	obs.Log.With(
		"productID", id,
	).Debug("Fetching product data")

	// Simulate DB fetch: if the ID starts with "missing-", return not found.
	if strings.HasPrefix(id, "missing-") {
		obs.Log.With("productID", id).Warn("Product not found in repository")
		return "", ErrProductNotFound
	}

	// Otherwise, return a dummy product with its ID.
	obs.Log.With("productID", id).Debug("Product found in repository")
	return fmt.Sprintf("Product ABC with ID %s", id), nil
}

func NewProductRepository() ProductRepository {
	return &productRepositoryImpl{}
}