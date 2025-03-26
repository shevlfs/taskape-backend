package handlers

import (
	"context"
	"fmt"
	"os"
	"time"

	pb "taskape-backend/proto"

	"github.com/golang-jwt/jwt"
)

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
		"exp":   time.Now().Add(time.Hour * 24).Unix(),
		"iat":   time.Now().Unix(),
		"type":  "access",
	}

	refreshClaims := jwt.MapClaims{
		"phone": phone,
		"exp":   time.Now().Add(time.Hour * 24 * 30).Unix(),
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

func (h *AuthHandler) LoginNewUser(ctx context.Context, req *pb.NewUserLoginRequest) (*pb.NewUserLoginResponse, error) {
	print("LoginNewUser called with phone: ", req.Phone)
	tx, err := h.Pool.Begin(ctx)
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
	var existingID int64 = -1
	if !profileExists {
		_, err = tx.Exec(ctx, "INSERT INTO users (phone) VALUES ($1)", req.Phone)
		if err != nil {
			return nil, fmt.Errorf("insert failed: %v", err)
		}
	} else {
		err = tx.QueryRow(ctx, "SELECT id FROM users WHERE phone = $1", req.Phone).Scan(&existingID)
		if err != nil {
			return nil, fmt.Errorf("failed to get user ID: %v", err)
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
		UserId:        existingID,
	}, nil
}

func (h *AuthHandler) ValidateToken(ctx context.Context, req *pb.ValidateTokenRequest) (*pb.ValidateTokenResponse, error) {
	token, err := jwt.Parse(req.Token, func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !token.Valid {
		return &pb.ValidateTokenResponse{Valid: false}, nil
	}

	return &pb.ValidateTokenResponse{Valid: true}, nil
}

func (h *AuthHandler) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
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

func (h *AuthHandler) VerifyUserToken(ctx context.Context, req *pb.VerifyUserRequest) (*pb.VerifyUserResponse, error) {
	return &pb.VerifyUserResponse{
		Success: true,
	}, nil
}
