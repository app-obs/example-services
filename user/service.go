package main

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type UserService interface {
	GetUserInfo(ctx context.Context, userID string) (string, error)
}

type userServiceImpl struct {
	repo UserRepository
}

func (s *userServiceImpl) GetUserInfo(ctx context.Context, userID string) (string, error) {
	obs := ObsFromCtx(ctx)
	ctx, span := obs.Trace.Start(ctx, "UserService.GetUserInfo", trace.WithAttributes(attribute.String("user.id", userID)))
	defer span.End()

	obs.Log.With(
		"userID", userID,
	).Debug("Processing request")

	userInfo, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		obs.Log.With(
			"userID", userID,
			"error", err,
		).Error("Error fetching user")
		return "", err
	}

	obs.Log.With(
		"userID", userID,
		"userInfo", userInfo,
	).Info("Successfully retrieved user info")
	return userInfo, nil
}

func NewUserService(repo UserRepository) UserService {
	return &userServiceImpl{repo: repo}
}
