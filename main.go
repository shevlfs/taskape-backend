package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	pb "taskape-server/taskape-proto"

	"github.com/golang-jwt/jwt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

type server struct {
	pb.BackendRequestsServer
	pool *pgxpool.Pool
}

func generateToken(phone string) (string, error) {
	err := godotenv.Load()
	if err != nil {
		log.Printf("Warning: Error loading .env file: %v\n", err)
	
	}

	secretKey := os.Getenv("JWT_SECRET")
	if secretKey == "" {
		return "", fmt.Errorf("JWT_SECRET environment variable not set")
	}

	claims := jwt.MapClaims{
		"phone": phone,
		"exp":   time.Now().Add(time.Hour * 24).Unix(),
		"iat":   time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secretKey))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func (s *server) LoginUser(ctx context.Context, req *pb.UserLoginRequest) (*pb.UserLoginResponse, error) {
	// checkQuery := `SELECT EXISTS(SELECT 1 FROM users WHERE phone = $1)`

	// var profileExists bool
	// err := s.pool.QueryRow(ctx, checkQuery, req.Phone).Scan(&profileExists)
	// if err != nil {
	// 	return nil, fmt.Errorf("database query failed: %v", err)
	// }
	token, err := generateToken(req.Phone)
	if err != nil {
		return nil, fmt.Errorf("token generation failed: %v", err)
	}

	println(token)
	return &pb.UserLoginResponse{Token: token}, nil
}

func main() {
	if err := godotenv.Load(); err != nil {
		panic(err)
	}

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Fatal("DB_URL environment variable not set")
	}

	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("Unable to parse pool config: %v\n", err)
	}

	poolConfig.MaxConns = 10
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		log.Fatalf("Unable to create connection pool: %v\n", err)
	}
	defer pool.Close()


	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}


	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterBackendRequestsServer(s, &server{pool: pool})

	log.Println("Server started on :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
