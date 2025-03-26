package config

import (
	"os"
	"strconv"

	"taskape-backend/db"
)

type Config struct {
	Server struct {
		Port string
	}

	Database db.Config

	JWT struct {
		Secret string
	}

	Debug bool
}

func Load() (*Config, error) {
	cfg := &Config{}

	cfg.Server.Port = getEnv("PORT", "50051")

	cfg.Database.Host = getEnv("DB_HOST", "localhost")
	cfg.Database.Port = getEnv("DB_PORT", "5432")
	cfg.Database.User = getEnv("DB_USER", "postgres")
	cfg.Database.Password = getEnv("DB_PASSWORD", "postgres")
	cfg.Database.Name = getEnv("DB_NAME", "taskape")
	cfg.Database.SSLMode = getEnv("DB_SSL_MODE", "disable")

	cfg.JWT.Secret = getEnv("JWT_SECRET", "your-secret-here")

	debug, _ := strconv.ParseBool(getEnv("DEBUG", "false"))
	cfg.Debug = debug

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	return defaultValue
}
