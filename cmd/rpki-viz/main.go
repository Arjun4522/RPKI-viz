package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rpki-viz/backend/internal/config"
	"github.com/rpki-viz/backend/internal/db"
	"github.com/rpki-viz/backend/internal/graphql"
	"github.com/rpki-viz/backend/internal/ingestor"
	"github.com/rpki-viz/backend/internal/validator"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize PostgreSQL client
	postgresClient, err := db.NewPostgresClient(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer postgresClient.Close()

	// Initialize Redis client
	redisClient, err := db.NewRedisClient(cfg.RedisURL)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	// Initialize GraphQL resolver
	resolver := graphql.NewResolver(postgresClient, redisClient)

	// Initialize ingestor
	ing := ingestor.NewIngestor(postgresClient)

	// Initialize prefix validator
	prefixValidator := validator.NewPrefixValidator()

	// Start background ingestion
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ticker := time.NewTicker(cfg.IngestionInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				log.Println("Starting data ingestion...")
				if err := ing.IngestFromAllSources(ctx); err != nil {
					log.Printf("Ingestion failed: %v", err)
				} else {
					log.Println("Data ingestion completed successfully")
				}
			}
		}
	}()

	// Start HTTP server
	srv := &http.Server{
		Addr:    cfg.ServerAddr,
		Handler: setupRoutes(resolver, postgresClient, prefixValidator),
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting server on %s", cfg.ServerAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Give outstanding requests 30 seconds to complete
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func validateAllPrefixes(pg *db.PostgresClient, pv *validator.PrefixValidator) {
	ctx := context.Background()
	prefixes, err := pg.GetAllPrefixes(ctx)
	if err != nil {
		log.Printf("Failed to get all prefixes: %v", err)
		return
	}
	log.Printf("Starting bulk validation of %d prefixes", len(prefixes))
	for i, p := range prefixes {
		if i%1000 == 0 {
			log.Printf("Validated %d/%d prefixes", i, len(prefixes))
		}
		asn, err := pg.GetASNByID(ctx, p.ASNID)
		if err != nil || asn == nil {
			continue
		}
		vrps, err := pg.GetVRPsByASN(ctx, p.ASNID)
		if err != nil {
			continue
		}
		result := pv.ValidatePrefix(asn.Number, p.CIDR, vrps)
		err = pg.UpdatePrefixValidationState(ctx, p.ID, result.State)
		if err != nil {
			log.Printf("Failed to update prefix %s: %v", p.ID, err)
		}
	}
	log.Println("Bulk validation completed")
}

func setupRoutes(resolver *graphql.Resolver, postgresClient *db.PostgresClient, prefixValidator *validator.PrefixValidator) http.Handler {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// Check database connectivity
		if err := postgresClient.HealthCheck(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(fmt.Sprintf("Database unhealthy: %v", err)))
			return
		}

		// Check Redis connectivity
		if redisClient := resolver.GetRedisClient(); redisClient != nil {
			if err := redisClient.HealthCheck(ctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(fmt.Sprintf("Redis unhealthy: %v", err)))
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// GraphQL endpoints
	graphqlHandler := graphql.GraphQLHandler(resolver)
	mux.Handle("/graphql", graphqlHandler)

	// GraphQL playground for development
	mux.HandleFunc("/playground", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/graphql", http.StatusFound)
	})

	// Bulk validation endpoint
	mux.HandleFunc("/validate-all", func(w http.ResponseWriter, r *http.Request) {
		go validateAllPrefixes(postgresClient, prefixValidator)
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("Bulk validation started in background"))
	})

	return mux
}
