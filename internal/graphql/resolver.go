package graphql

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

import (
	"github.com/rpki-viz/backend/internal/db"
	"github.com/rpki-viz/backend/internal/service"
	"github.com/rpki-viz/backend/internal/validator"
)

type Resolver struct {
	postgresClient  *db.PostgresClient
	redisClient     *db.RedisClient
	roaProcessor    *service.ROAProcessor
	prefixValidator *validator.PrefixValidator
}

// NewResolver creates a new GraphQL resolver with dependencies
func NewResolver(postgresClient *db.PostgresClient, redisClient *db.RedisClient) *Resolver {
	return &Resolver{
		postgresClient:  postgresClient,
		redisClient:     redisClient,
		roaProcessor:    service.NewROAProcessor(),
		prefixValidator: validator.NewPrefixValidator(),
	}
}

// GetRedisClient returns the Redis client
func (r *Resolver) GetRedisClient() *db.RedisClient {
	return r.redisClient
}

// GetPostgresClient returns the PostgreSQL client
func (r *Resolver) GetPostgresClient() *db.PostgresClient {
	return r.postgresClient
}
