package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// Config holds all application configuration values.
type Config struct {
	ChunkSize    int    `json:"chunk_size"`
	ChunkOverlap int    `json:"chunk_overlap"`
	ModelPath    string `json:"model_path"`
	Workers      int    `json:"workers"`
	BatchSize    int    `json:"batch_size"`
	MaxCPU       int    `json:"max_cpu"`
	MaxMem       int    `json:"max_mem"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	// Default to half of available cores, min 1
	defaultCPU := runtime.NumCPU() / 2
	if defaultCPU < 1 {
		defaultCPU = 1
	}

	return &Config{
		ChunkSize:    500,
		ChunkOverlap: 50,
		ModelPath:    "",
		Workers:      defaultCPU,
		BatchSize:    100,
		MaxCPU:       defaultCPU,
		MaxMem:       512,
	}
}

// LoadFromFile reads a JSON config file and merges it with defaults.
// Missing fields in the file retain their default values.
func LoadFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err // Return defaults if file doesn't exist
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// DefaultConfigPath returns ~/.vecbound/config.json.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(home, ".vecbound", "config.json")
}
