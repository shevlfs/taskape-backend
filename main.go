package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	pb "taskape-server/proto"

	"github.com/golang-jwt/jwt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var publicEndpoints = map[string]bool{
	"/taskapebackend.BackendRequests/loginNewUser":  true,
	"/taskapebackend.BackendRequests/validateToken": true,
	"/taskapebackend.BackendRequests/refreshToken":  true,
}

type server struct {
	pb.BackendRequestsServer
	pool *pgxpool.Pool
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

func generateTokens(phone string) (*TokenPair, error) {
	secretKey := os.Getenv("JWT_SECRET")
	if secretKey == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable not set")
	}

	accessClaims := jwt.MapClaims{
		"phone": phone,
		"exp":   time.Now().Add(time.Minute * 15).Unix(),
		"iat":   time.Now().Unix(),
		"type":  "access",
	}

	refreshClaims := jwt.MapClaims{
		"phone": phone,
		"exp":   time.Now().Add(time.Hour * 24 * 7).Unix(),
		"iat":   time.Now().Unix(),
		"type":  "refresh",
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)

	accessTokenString, err := accessToken.SignedString([]byte(secretKey))
	if err != nil {
		return nil, err
	}

	refreshTokenString, err := refreshToken.SignedString([]byte(secretKey))
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
	}, nil
}

func (s *server) LoginNewUser(ctx context.Context, req *pb.NewUserLoginRequest) (*pb.NewUserLoginResponse, error) {
	print("LoginNewUser called with phone: ", req.Phone)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	checkQuery := `SELECT EXISTS(SELECT 1 FROM users WHERE phone = $1 AND handle IS NOT NULL)`
	var profileExists bool
	err = tx.QueryRow(ctx, checkQuery, req.Phone).Scan(&profileExists)
	if err != nil {
		return nil, fmt.Errorf("database query failed: %v", err)
	}

	if !profileExists {
		_, err = tx.Exec(ctx, "INSERT INTO users (phone) VALUES ($1)", req.Phone)
		if err != nil {
			return nil, fmt.Errorf("insert failed: %v", err)
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	tokens, err := generateTokens(req.Phone)
	if err != nil {
		return nil, fmt.Errorf("token generation failed: %v", err)
	}

	return &pb.NewUserLoginResponse{
		Token:         tokens.AccessToken,
		RefreshToken:  tokens.RefreshToken,
		ProfileExists: profileExists,
	}, nil
}

func (s *server) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	token, err := jwt.Parse(req.Token, func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !token.Valid {
		return &pb.ValidateTokenResponse{Valid: false}, nil
	}

	return &pb.ValidateTokenResponse{Valid: true}, nil
}

func (s *server) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	refreshToken, err := jwt.Parse(req.RefreshToken, func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !refreshToken.Valid {
		return nil, fmt.Errorf("invalid refresh token")
	}

	claims, ok := refreshToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	phone, ok := claims["phone"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid phone in token")
	}

	newTokens, err := generateTokens(phone)
	if err != nil {
		return nil, fmt.Errorf("token generation failed: %v", err)
	}

	return &pb.RefreshTokenResponse{
		Token:        newTokens.AccessToken,
		RefreshToken: newTokens.RefreshToken,
	}, nil
}

func AuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	println(info.FullMethod)
	if publicEndpoints[info.FullMethod] {
		return handler(ctx, req)
	}

	print("authinterceptor is checking auth info\n")

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "no metadata")
	}

	values := md["authorization"]
	if len(values) == 0 {
		return nil, status.Errorf(codes.Unauthenticated, "no auth token")
	}

	token, err := jwt.Parse(values[0], func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !token.Valid {
		return nil, status.Errorf(codes.Unauthenticated, "invalid token")
	}
	return handler(ctx, req)
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

	s := grpc.NewServer(grpc.UnaryInterceptor(AuthInterceptor))
	pb.RegisterBackendRequestsServer(s, &server{pool: pool})

	log.Println("Server started on :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
