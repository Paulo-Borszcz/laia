package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	NexusBaseURL  string
	NexusAppToken string

	WAPhoneNumberID string
	WAAccessToken   string
	WAVerifyToken   string

	GeminiAPIKey string

	BaseURL string
	Port    string
}

func Load() (*Config, error) {
	// .env is optional — env vars may already be set (e.g. in production)
	_ = godotenv.Load()

	cfg := &Config{
		NexusBaseURL:    os.Getenv("NEXUS_BASE_URL"),
		NexusAppToken:   os.Getenv("NEXUS_APP_TOKEN"),
		WAPhoneNumberID: os.Getenv("WA_PHONE_NUMBER_ID"),
		WAAccessToken:   os.Getenv("WA_ACCESS_TOKEN"),
		WAVerifyToken:   os.Getenv("WA_VERIFY_TOKEN"),
		GeminiAPIKey:    os.Getenv("GEMINI_API_KEY"),
		BaseURL:         os.Getenv("BASE_URL"),
		Port:            os.Getenv("PORT"),
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = fmt.Sprintf("http://localhost:%s", cfg.Port)
	}

	if cfg.WAVerifyToken == "" {
		token, err := randomHex(16)
		if err != nil {
			return nil, fmt.Errorf("generating verify token: %w", err)
		}
		cfg.WAVerifyToken = token
	}

	for _, req := range []struct {
		name, val string
	}{
		{"NEXUS_BASE_URL", cfg.NexusBaseURL},
		{"NEXUS_APP_TOKEN", cfg.NexusAppToken},
		{"WA_PHONE_NUMBER_ID", cfg.WAPhoneNumberID},
		{"WA_ACCESS_TOKEN", cfg.WAAccessToken},
		{"GEMINI_API_KEY", cfg.GeminiAPIKey},
	} {
		if req.val == "" {
			return nil, fmt.Errorf("required env var %s is not set", req.name)
		}
	}

	return cfg, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
