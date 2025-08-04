package main

import (
	"context"

	"github.com/app-obs/go/observability"
)

type UserService interface {
	GetUserInfo(ctx context.Context, obs *observability.Observability, userID string) (string, error)
}

type userServiceImpl struct {
	repo UserRepository
}

func (s *userServiceImpl) GetUserInfo(ctx context.Context, obs *observability.Observability, userID string) (string, error) {
	ctx, obs, span := observability.StartSpanFromCtx(ctx, "UserService.GetUserInfo", observability.SpanAttributes{"user.id": userID})
	defer span.End()

	obs.Log.With(
		"userID", userID,
	).Debug("Processing request")

	userInfo, err := s.repo.GetUserByID(ctx, obs, userID)
	if err != nil {
		obs.ErrorHandler.Record(err, "Error fetching user")
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