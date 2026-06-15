package utils

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var jwtSecretKey []byte

func init() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "supersecretsigningkey123!"
	}
	jwtSecretKey = []byte(secret)
}

// HashPassword hashes a plain-text password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPasswordHash compares a plain password against its bcrypt hash
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateJWT creates a signed access token containing user metadata
func GenerateJWT(userID string, role string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  userID,
		"role": role,
		"exp":  time.Now().Add(15 * time.Minute).Unix(),
		"iat":  time.Now().Unix(),
	})

	tokenString, err := token.SignedString(jwtSecretKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// ParseJWT decodes and verifies a JWT token
func ParseJWT(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecretKey, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token claims")
}

// Structures for Google JWKS verification
type JWK struct {
	Kty string   `json:"kty"`
	Alg string   `json:"alg"`
	Use string   `json:"use"`
	Kid string   `json:"kid"`
	N   string   `json:"n"`
	E   string   `json:"e"`
}

type JWKS struct {
	Keys []JWK `json:"keys"`
}

var (
	jwkCache     *JWKS
	jwkCacheTime time.Time
	jwkMutex     sync.RWMutex
)

func fetchGoogleJWKS() (*JWKS, error) {
	jwkMutex.RLock()
	if jwkCache != nil && time.Since(jwkCacheTime) < 1*time.Hour {
		defer jwkMutex.RUnlock()
		return jwkCache, nil
	}
	jwkMutex.RUnlock()

	jwkMutex.Lock()
	defer jwkMutex.Unlock()

	// Double check
	if jwkCache != nil && time.Since(jwkCacheTime) < 1*time.Hour {
		return jwkCache, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/certs")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, err
	}

	jwkCache = &jwks
	jwkCacheTime = time.Now()
	return jwkCache, nil
}

// VerifyGoogleIDToken verifies the Google JWT ID token using official Google JWKS certs
func VerifyGoogleIDToken(tokenStr string, clientID string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("missing kid header")
		}

		jwks, err := fetchGoogleJWKS()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch Google JWKS: %w", err)
		}

		var targetKey *JWK
		for _, key := range jwks.Keys {
			if key.Kid == kid {
				targetKey = &key
				break
			}
		}

		if targetKey == nil {
			return nil, errors.New("signature key matching kid not found")
		}

		nBytes, err := base64.RawURLEncoding.DecodeString(targetKey.N)
		if err != nil {
			return nil, fmt.Errorf("failed to decode JWK modulus N: %w", err)
		}

		eBytes, err := base64.RawURLEncoding.DecodeString(targetKey.E)
		if err != nil {
			return nil, fmt.Errorf("failed to decode JWK exponent E: %w", err)
		}

		var eVal int
		for _, b := range eBytes {
			eVal = (eVal << 8) | int(b)
		}

		pubKey := &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: eVal,
		}

		return pubKey, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid Google ID Token signature")
	}

	// Verify Issuer
	iss, _ := claims["iss"].(string)
	if !strings.Contains(iss, "accounts.google.com") {
		return nil, errors.New("invalid token issuer")
	}

	// Verify Audience
	if clientID != "" {
		aud, _ := claims["aud"].(string)
		if aud != clientID {
			return nil, errors.New("invalid token audience")
		}
	}

	return claims, nil
}
