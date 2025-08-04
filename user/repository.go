package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/app-obs/go/observability"
)

// ErrUserNotFound is returned when a user is not found.
var ErrUserNotFound = errors.New("user not found")

type UserRepository interface {
	GetUserByID(ctx context.Context, obs *observability.Observability, id string) (string, error)
}

type userRepositoryImpl struct{}

func (r *userRepositoryImpl) GetUserByID(ctx context.Context, obs *observability.Observability, id string) (string, error) {
	ctx, obs, span := observability.StartSpanFromCtx(ctx, "UserRepository.GetUserByID", observability.SpanAttributes{"user.id": id})
	defer span.End()

	obs.Log.With(
		"userID", id,
	).Debug("Fetching user data")

	// Simulate DB fetch: if the ID starts with "missing-", return not found.
	if strings.HasPrefix(id, "missing-") {
		obs.Log.With("userID", id).Warn("User not found in repository")
		return "", ErrUserNotFound
	}

	// Otherwise, return a dummy user with its ID.
	obs.Log.With("userID", id).Debug("User found in repository")
	return fmt.Sprintf("User ABC with ID %s", id), nil
}

func NewUserRepository() UserRepository {
	return &userRepositoryImpl{}
}