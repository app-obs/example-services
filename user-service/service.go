package main

import (
	"context"
	"log"

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
	ctx, span := tracer.Start(ctx, "UserService.GetUserInfo", trace.WithAttributes(attribute.String("user.id", userID)))
	defer span.End()

	traceID, spanID := getTraceSpanInfo(ctx)
	log.Printf("[%s|%s] UserService: Processing request for user ID %s", traceID, spanID, userID)

	userInfo, err := s.repo.FetchUser(ctx, userID)
	if err != nil {
		log.Printf("[%s|%s] UserService: Error fetching user %s: %v", traceID, spanID, userID, err)
		return "", err
	}

	log.Printf("[%s|%s] UserService: Successfully retrieved user info for %s", traceID, spanID, userID)
	return userInfo, nil
}

func NewUserService(repo UserRepository) UserService {
	return &userServiceImpl{repo: repo}
}
