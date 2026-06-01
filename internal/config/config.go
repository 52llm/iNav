package config

import (
	"errors"
	"os"
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	Token      string
	DBPath     string
	ListenAddr string
	PublicRead bool

	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string
}

// Load reads configuration from environment variables, applying defaults.
func Load() (Config, error) {
	c := Config{
		Token:      os.Getenv("INAV_TOKEN"),
		DBPath:     envOr("INAV_DB_PATH", "inav.db"),
		ListenAddr: envOr("INAV_LISTEN_ADDR", ":8080"),
		PublicRead: os.Getenv("INAV_PUBLIC_READ") == "true",
		LLMBaseURL: os.Getenv("INAV_LLM_BASE_URL"),
		LLMAPIKey:  os.Getenv("INAV_LLM_API_KEY"),
		LLMModel:   os.Getenv("INAV_LLM_MODEL"),
	}
	if c.Token == "" {
		return Config{}, errors.New("INAV_TOKEN is required")
	}
	return c, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
