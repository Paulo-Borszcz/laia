package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	NexusBaseURL      string
	NexusAppToken     string
	NexusAdminToken   string
	NexusAdminProfile int

	WAPhoneNumberID string
	WAAccessToken   string
	WAVerifyToken   string

	OpenAIAPIKey string

	BaseURL string
	Port    string
	DataDir string
}

func Load() (*Config, error) {
	// .env is optional — env vars may already be set (e.g. in production)
	_ = godotenv.Load()

	cfg := &Config{
		NexusBaseURL:    os.Getenv("NEXUS_BASE_URL"),
		NexusAppToken:   os.Getenv("NEXUS_APP_TOKEN"),
		NexusAdminToken:   os.Getenv("NEXUS_ADMIN_TOKEN"),
		NexusAdminProfile: parseIntEnv("NEXUS_ADMIN_PROFILE"),
		WAPhoneNumberID:   os.Getenv("WA_PHONE_NUMBER_ID"),
		WAAccessToken:   os.Getenv("WA_ACCESS_TOKEN"),
		WAVerifyToken:   os.Getenv("WA_VERIFY_TOKEN"),
		OpenAIAPIKey:    os.Getenv("OPENAI_API_KEY"),
		BaseURL:         os.Getenv("BASE_URL"),
		Port:            os.Getenv("PORT"),
		DataDir:         os.Getenv("DATA_DIR"),
	}

	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	if cfg.DataDir == "" {
		cfg.DataDir = "."
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
		{"OPENAI_API_KEY", cfg.OpenAIAPIKey},
	} {
		if req.val == "" {
			return nil, fmt.Errorf("required env var %s is not set", req.name)
		}
	}

	return cfg, nil
}

func parseIntEnv(key string) int {
	v, _ := strconv.Atoi(os.Getenv(key))
	return v
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
