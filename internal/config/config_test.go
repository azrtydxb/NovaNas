package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("LISTEN_ADDR", ":8080")

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
	if _, err := Load(); err == nil {
		t.Fatal("expected error for missing DATABASE_URL")
	}
}
