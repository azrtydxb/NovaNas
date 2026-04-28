package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("LISTEN_ADDR", ":8080")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DatabaseURL != "postgres://x" {
		t.Errorf("DatabaseURL=%q", cfg.DatabaseURL)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr=%q", cfg.ListenAddr)
	}
	if cfg.RedisURL != "redis://localhost:6379/0" {
		t.Errorf("RedisURL=%q", cfg.RedisURL)
	}
	if cfg.ZFSBin != "/sbin/zfs" {
		t.Errorf("ZFSBin default=%q", cfg.ZFSBin)
	}
	if cfg.ZpoolBin != "/sbin/zpool" {
		t.Errorf("ZpoolBin default=%q", cfg.ZpoolBin)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel default=%q", cfg.LogLevel)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("LISTEN_ADDR", ":8080")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}

	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing LISTEN_ADDR")
	}

	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("LISTEN_ADDR", ":8080")
	t.Setenv("REDIS_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing REDIS_URL")
	}
}
