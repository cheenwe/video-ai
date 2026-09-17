package security

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	SessionVersion int    `json:"session_version"`
	UserID         uint   `json:"user_id"`
	Username       string `json:"username"`
	Role           string `json:"role"`
	Purpose        string `json:"purpose,omitempty"`
	jwt.RegisteredClaims
}

func CreateToken(secret, algorithm string, expireDays int, userID uint, username, role string, versions ...int) (string, string, error) {
	return createToken(secret, algorithm, expireDays*24*60, userID, username, role, "", versions...)
}

func CreatePurposeToken(secret, algorithm string, expireMinutes int, userID uint, username, role, purpose string, versions ...int) (string, string, error) {
	return createToken(secret, algorithm, expireMinutes, userID, username, role, purpose, versions...)
}

func createToken(secret, algorithm string, expireMinutes int, userID uint, username, role, purpose string, versions ...int) (string, string, error) {
	jti := uuid.New().String()
	now := time.Now()
	exp := now.Add(time.Duration(expireMinutes) * time.Minute)
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		Purpose:  purpose,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	if len(versions) > 0 {
		claims.SessionVersion = versions[0]
	}
	method := jwt.SigningMethodHS256
	if algorithm != "" && algorithm != "HS256" {
		if m := jwt.GetSigningMethod(algorithm); m != nil {
			token := jwt.NewWithClaims(m, claims)
			signed, err := token.SignedString([]byte(secret))
			return signed, jti, err
		}
	}
	token := jwt.NewWithClaims(method, claims)
	signed, err := token.SignedString([]byte(secret))
	return signed, jti, err
}

func ParseToken(secret, algorithm, tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{algorithm}))
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}
