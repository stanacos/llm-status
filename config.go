package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configDirName  = ".claude-status"
	configFileName = "config.json"
)

type appConfig struct {
	LastProvider ProviderID `json:"last_provider"`
}

func loadLastProvider() (ProviderID, error) {
	path, err := appConfigPath()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read config: %w", err)
	}

	var cfg appConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse config: %w", err)
	}

	if !isValidProvider(cfg.LastProvider) {
		return "", nil
	}
	return cfg.LastProvider, nil
}

func saveLastProvider(provider ProviderID) error {
	if !isValidProvider(provider) {
		return fmt.Errorf("invalid provider %q", provider)
	}

	path, err := appConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	content, err := json.MarshalIndent(appConfig{LastProvider: provider}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	content = append(content, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp config: %w", err)
	}
	return nil
}

func appConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, configDirName, configFileName), nil
}
