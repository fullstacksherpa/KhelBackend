package auth

import "github.com/golang-jwt/jwt/v5"

type Authenticator interface {
	GenerateTokens(userID int64, role string) (string, string, error)
	ValidateAccessToken(token string) (*jwt.Token, error)
	ValidateRefreshToken(token string) (*jwt.Token, error)
}
