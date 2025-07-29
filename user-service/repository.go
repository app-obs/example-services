package main

import (
	"context"
	"log"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type UserRepository interface {
	FetchUser(ctx context.Context, userID string) (string, error)
}

type userRepositoryImpl struct{}

func (r *userRepositoryImpl) FetchUser(ctx context.Context, userID string) (string, error) {
	ctx, span := tracer.Start(ctx, "UserRepository.FetchUser", trace.WithAttributes(attribute.String("user.id", userID)))
	defer span.End()

	traceID, spanID := getTraceSpanInfo(ctx)
	log.Printf("[%s|%s] UserRepository: Fetching user data for ID %s", traceID, spanID, userID)

	// Simulate DB fetch
	userInfo := "Nama: User " + userID + ", Email: user" + userID + "@example.com"

	log.Printf("[%s|%s] UserRepository: Successfully fetched user data for ID %s", traceID, spanID, userID)
	return userInfo, nil
}

func NewUserRepository() UserRepository {
	return &userRepositoryImpl{}
}
