package middleware

import (
	"context"

	"ride-hail-system/internal/domain/models"
	"ride-hail-system/pkg/logger"
)

type (
	AuthService interface {
		RoleCheck(ctx context.Context, token string) (*models.User, error)
	}

	Middleware struct {
		auth AuthService
		log  logger.Logger
	}
)

func NewMiddleware(auth AuthService, log logger.Logger) *Middleware {
	return &Middleware{
		auth: auth,
		log:  log,
	}
}
