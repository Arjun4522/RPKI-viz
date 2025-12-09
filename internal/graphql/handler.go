package graphql

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
)

// NewHandler creates a new GraphQL HTTP handler
func NewHandler(resolver *Resolver) http.Handler {
	// Create the GraphQL schema
	srv := handler.NewDefaultServer(NewExecutableSchema(Config{
		Resolvers: resolver,
	}))

	mux := http.NewServeMux()

	// GraphQL endpoint
	mux.Handle("/graphql", srv)

	// GraphQL playground (for development)
	mux.Handle("/playground", playground.Handler("RPKI Visualization", "/graphql"))

	return mux
}

// GraphQLHandler creates a GraphQL handler with proper error handling
func GraphQLHandler(resolver *Resolver) http.Handler {
	schema := NewExecutableSchema(Config{
		Resolvers: resolver,
	})

	h := handler.NewDefaultServer(schema)

	// Add custom error handling if needed
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers for development
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		h.ServeHTTP(w, r)
	})
}
