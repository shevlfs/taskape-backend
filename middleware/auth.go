package middleware

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/golang-jwt/jwt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var publicEndpoints = []string{
	"/taskapebackend.BackendRequests/loginNewUser",
	"/taskapebackend.BackendRequests/validateToken",
	"/taskapebackend.BackendRequests/refreshToken",
}

func AuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	fmt.Println(info.FullMethod)

	if slices.Contains(publicEndpoints, info.FullMethod) {
		return handler(ctx, req)
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		fmt.Print("unauthenticated request rejected\n")
		return nil, status.Errorf(codes.Unauthenticated, "no metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		fmt.Print("unauthenticated request rejected, no token\n")
		return nil, status.Errorf(codes.Unauthenticated, "no auth token")
	}

	token, err := jwt.Parse(values[0], func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !token.Valid {
		fmt.Print("unauthenticated request rejected, invalid token\n")
		return nil, status.Errorf(codes.Unauthenticated, "invalid token")
	}

	return handler(ctx, req)
}
