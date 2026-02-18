package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLastProviderPrefersNewConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	newPath, err := appConfigPath()
	if err != nil {
		t.Fatalf("appConfigPath: %v", err)
	}
	legacyPath, err := legacyAppConfigPath()
	if err != nil {
		t.Fatalf("legacyAppConfigPath: %v", err)
	}

	writeTestFile(t, newPath, "{\n  \"last_provider\": \"codex\"\n}\n")
	writeTestFile(t, legacyPath, "{\n  \"last_provider\": \"claude\"\n}\n")

	provider, err := loadLastProvider()
	if err != nil {
		t.Fatalf("loadLastProvider: %v", err)
	}
	if provider != ProviderCodex {
		t.Fatalf("provider=%q want=%q", provider, ProviderCodex)
	}
}

func TestLoadLastProviderMigratesLegacyConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	newPath, err := appConfigPath()
	if err != nil {
		t.Fatalf("appConfigPath: %v", err)
	}
	legacyPath, err := legacyAppConfigPath()
	if err != nil {
		t.Fatalf("legacyAppConfigPath: %v", err)
	}

	const legacyContent = "{\n  \"last_provider\": \"claude\"\n}\n"
	writeTestFile(t, legacyPath, legacyContent)

	provider, err := loadLastProvider()
	if err != nil {
		t.Fatalf("loadLastProvider: %v", err)
	}
	if provider != ProviderClaude {
		t.Fatalf("provider=%q want=%q", provider, ProviderClaude)
	}

	migrated, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	if string(migrated) != legacyContent {
		t.Fatalf("migrated content mismatch got=%q want=%q", string(migrated), legacyContent)
	}
}

func TestLoadLastProviderLegacyMigrationFailureIsNonFatal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	legacyPath, err := legacyAppConfigPath()
	if err != nil {
		t.Fatalf("legacyAppConfigPath: %v", err)
	}
	writeTestFile(t, legacyPath, "{\n  \"last_provider\": \"codex\"\n}\n")

	blockingDirPath := filepath.Join(home, configDirName)
	if err := os.MkdirAll(blockingDirPath, 0o755); err != nil {
		t.Fatalf("mkdir blocking dir: %v", err)
	}
	if err := os.Chmod(blockingDirPath, 0o555); err != nil {
		t.Fatalf("chmod blocking dir: %v", err)
	}
	defer os.Chmod(blockingDirPath, 0o755)

	provider, err := loadLastProvider()
	if err != nil {
		t.Fatalf("loadLastProvider: %v", err)
	}
	if provider != ProviderCodex {
		t.Fatalf("provider=%q want=%q", provider, ProviderCodex)
	}

	newPath := filepath.Join(home, configDirName, configFileName)
	_, err = os.Stat(newPath)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no migrated config at %s, got err=%v", newPath, err)
	}
}

func TestLoadLastProviderDoesNotFallbackWhenNewConfigExists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	newPath, err := appConfigPath()
	if err != nil {
		t.Fatalf("appConfigPath: %v", err)
	}
	legacyPath, err := legacyAppConfigPath()
	if err != nil {
		t.Fatalf("legacyAppConfigPath: %v", err)
	}

	writeTestFile(t, newPath, "{\n  \"last_provider\": \"unknown\"\n}\n")
	writeTestFile(t, legacyPath, "{\n  \"last_provider\": \"claude\"\n}\n")

	provider, err := loadLastProvider()
	if err != nil {
		t.Fatalf("loadLastProvider: %v", err)
	}
	if provider != "" {
		t.Fatalf("provider=%q want empty", provider)
	}
}

func TestSaveLastProviderWritesOnlyNewConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	legacyPath, err := legacyAppConfigPath()
	if err != nil {
		t.Fatalf("legacyAppConfigPath: %v", err)
	}
	writeTestFile(t, legacyPath, "{\n  \"last_provider\": \"claude\"\n}\n")

	if err := saveLastProvider(ProviderCodex); err != nil {
		t.Fatalf("saveLastProvider: %v", err)
	}

	newPath, err := appConfigPath()
	if err != nil {
		t.Fatalf("appConfigPath: %v", err)
	}
	newCfg := readTestConfig(t, newPath)
	if newCfg.LastProvider != ProviderCodex {
		t.Fatalf("new config provider=%q want=%q", newCfg.LastProvider, ProviderCodex)
	}

	legacyCfg := readTestConfig(t, legacyPath)
	if legacyCfg.LastProvider != ProviderClaude {
		t.Fatalf("legacy config provider=%q want=%q", legacyCfg.LastProvider, ProviderClaude)
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readTestConfig(t *testing.T, path string) appConfig {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var cfg appConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return cfg
}
