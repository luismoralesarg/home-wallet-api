package jwt

import (
	"errors"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

type Issuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func New(secret string, ttl time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), ttl: ttl, now: time.Now}
}

func (i *Issuer) Issue(userID string) (string, time.Time, error) {
	now := i.now()
	exp := now.Add(i.ttl)
	claims := gojwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  gojwt.NewNumericDate(now),
		ExpiresAt: gojwt.NewNumericDate(exp),
	}
	tok, err := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims).SignedString(i.secret)
	return tok, exp, err
}

func (i *Issuer) Parse(token string) (string, error) {
	claims := &gojwt.RegisteredClaims{}
	_, err := gojwt.ParseWithClaims(token, claims, func(*gojwt.Token) (any, error) {
		return i.secret, nil
	}, gojwt.WithValidMethods([]string{"HS256"}), gojwt.WithExpirationRequired())
	if err != nil {
		return "", err
	}
	if claims.Subject == "" {
		return "", errors.New("token sin subject")
	}
	return claims.Subject, nil
}
