package testutil

import (
	"os"
	"testing"
)

const (
	testDBDefaultUser     = "merrymaker"
	testDBDefaultPassword = "merrymaker"
	testDBDefaultName     = "merrymaker"
)

func TestDefaultTestDBConfig(t *testing.T) {
	// Save original env vars
	origHost := os.Getenv("TEST_DB_HOST")
	origPort := os.Getenv("TEST_DB_PORT")
	origUser := os.Getenv("TEST_DB_USER")
	origPassword := os.Getenv("TEST_DB_PASSWORD")
	origName := os.Getenv("TEST_DB_NAME")

	// Restore env vars after test
	defer func() {
		setOrUnset("TEST_DB_HOST", origHost)
		setOrUnset("TEST_DB_PORT", origPort)
		setOrUnset("TEST_DB_USER", origUser)
		setOrUnset("TEST_DB_PASSWORD", origPassword)
		setOrUnset("TEST_DB_NAME", origName)
	}()

	t.Run("defaults to local test database port 55432", func(t *testing.T) {
		testDefaultConfig(t)
	})

	t.Run("respects TEST_DB_PORT environment variable", func(t *testing.T) {
		testCIConfig(t)
	})
}

func testDefaultConfig(t *testing.T) {
	t.Helper()
	// Clear all env vars to test defaults
	os.Unsetenv("TEST_DB_HOST")
	os.Unsetenv("TEST_DB_PORT")
	os.Unsetenv("TEST_DB_USER")
	os.Unsetenv("TEST_DB_PASSWORD")
	os.Unsetenv("TEST_DB_NAME")

	cfg := DefaultTestDBConfig()

	if cfg.Host != "localhost" {
		t.Errorf("expected Host=localhost, got %s", cfg.Host)
	}
	if cfg.Port != "55432" {
		t.Errorf("expected Port=55432 (test DB), got %s", cfg.Port)
	}
	if cfg.User != testDBDefaultUser {
		t.Errorf("expected User=%s, got %s", testDBDefaultUser, cfg.User)
	}
	if cfg.Password != testDBDefaultPassword {
		t.Errorf("expected Password=%s, got %s", testDBDefaultPassword, cfg.Password)
	}
	if cfg.DBName != testDBDefaultName {
		t.Errorf("expected DBName=%s, got %s", testDBDefaultName, cfg.DBName)
	}
}

func testCIConfig(t *testing.T) {
	t.Helper()
	// Simulate CI/CD environment with port 5432
	os.Setenv("TEST_DB_HOST", "postgres")
	os.Setenv("TEST_DB_PORT", "5432")
	os.Setenv("TEST_DB_USER", testDBDefaultUser)
	os.Setenv("TEST_DB_PASSWORD", testDBDefaultPassword)
	os.Setenv("TEST_DB_NAME", testDBDefaultName)

	cfg := DefaultTestDBConfig()

	if cfg.Host != "postgres" {
		t.Errorf("expected Host=postgres, got %s", cfg.Host)
	}
	if cfg.Port != "5432" {
		t.Errorf("expected Port=5432 (CI DB), got %s", cfg.Port)
	}
	if cfg.User != testDBDefaultUser {
		t.Errorf("expected User=%s, got %s", testDBDefaultUser, cfg.User)
	}
	if cfg.Password != testDBDefaultPassword {
		t.Errorf("expected Password=%s, got %s", testDBDefaultPassword, cfg.Password)
	}
	if cfg.DBName != testDBDefaultName {
		t.Errorf("expected DBName=%s, got %s", testDBDefaultName, cfg.DBName)
	}
}

func setOrUnset(key, value string) {
	if value == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, value)
	}
}
