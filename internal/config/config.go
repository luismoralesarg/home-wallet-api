package config

import (
	"errors"
	"os"
	"time"
)

type Config struct {
	DatabaseURL string
	JWTSecret   string
	Port        string
	JWTTTL      time.Duration
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Port:        getenv("PORT", "8080"),
		JWTTTL:      24 * time.Hour,
	}
	if v := os.Getenv("JWT_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return c, errors.New("JWT_TTL inválido (ej: 24h)")
		}
		c.JWTTTL = d
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL es obligatoria")
	}
	if len(c.JWTSecret) < 16 {
		return c, errors.New("JWT_SECRET es obligatoria (mínimo 16 caracteres)")
	}
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
