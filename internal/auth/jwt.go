package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTAuthenticator struct {
	refreshSecret string
	secret        string
	aud           string
	iss           string
}

func NewJWTAuthenticator(secret, refreshSecret, aud, iss string) *JWTAuthenticator {
	return &JWTAuthenticator{secret, refreshSecret, iss, aud}
}

// GenerateTokens generates both access and refresh tokens
func (a *JWTAuthenticator) GenerateTokens(userID int64, role string) (string, string, error) {
	accessClaims := jwt.MapClaims{
		"sub":  userID,
		"role": role,
		"exp":  time.Now().Add(time.Hour * 24 * 3).Unix(), // 3 days
		"iat":  time.Now().Unix(),
		"nbf":  time.Now().Unix(),
		"iss":  a.iss,
		"aud":  a.aud,
	}

	refreshClaims := jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour * 24 * 9).Unix(), // 9 days
		"iat": time.Now().Unix(),
		"iss": a.iss,
	}

	accessToken, err := a.generateTokenWithClaims(accessClaims, a.secret)
	if err != nil {
		return "", "", err
	}

	refreshToken, err := a.generateTokenWithClaims(refreshClaims, a.refreshSecret)
	if err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

func (a *JWTAuthenticator) generateTokenWithClaims(claims jwt.Claims, secret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// ValidateAccessToken validates the access token
func (a *JWTAuthenticator) ValidateAccessToken(token string) (*jwt.Token, error) {
	return jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return []byte(a.secret), nil
	}, jwt.WithExpirationRequired(), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))
}

// ValidateRefreshToken validates the refresh token
func (a *JWTAuthenticator) ValidateRefreshToken(token string) (*jwt.Token, error) {
	return jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return []byte(a.refreshSecret), nil
	}, jwt.WithExpirationRequired(), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))
}

func (a *JWTAuthenticator) Secret() string {
	return a.secret
}
