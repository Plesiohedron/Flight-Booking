package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	flightv1 "github.com/soa/flight-service/gen/flight/v1"
	"github.com/soa/flight-service/internal/cache"
	"github.com/soa/flight-service/internal/repository"
	"github.com/soa/flight-service/internal/server"
)

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func main() {
	databaseURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/flights?sslmode=disable")
	grpcPort := getEnv("GRPC_PORT", "50051")
	redisURL := getEnv("REDIS_URL", "redis://redis:6379")
	apiKey := getEnv("API_KEY", "")

	// Connect to PostgreSQL
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	log.Println("Connected to PostgreSQL")

	// Run migrations
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatalf("failed to create migration driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file:///migrations", "postgres", driver)
	if err != nil {
		log.Fatalf("failed to create migrator: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("failed to run migrations: %v", err)
	}
	log.Println("Migrations applied")

	// Connect to Redis
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("failed to parse Redis URL: %v", err)
	}
	redisClient := redis.NewClient(opts)
	log.Println("Connected to Redis")

	// Build dependencies
	repo := repository.New(db)
	redisCache := cache.New(redisClient)
	svc := server.New(repo, redisCache)

	// Build gRPC server with optional auth interceptor
	var grpcServer *grpc.Server
	if apiKey != "" {
		authInterceptor := server.NewAuthInterceptor(apiKey)
		grpcServer = grpc.NewServer(
			grpc.UnaryInterceptor(authInterceptor.Unary()),
		)
		log.Printf("Auth interceptor enabled (API key required)")
	} else {
		grpcServer = grpc.NewServer()
		log.Println("Auth interceptor disabled (no API_KEY set)")
	}

	flightv1.RegisterFlightServiceServer(grpcServer, svc)
	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	log.Printf("gRPC server listening on :%s", grpcPort)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
