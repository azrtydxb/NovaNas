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

func TestLoad_TLSDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("LISTEN_ADDR", ":8080")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TLS.CertDir != "/etc/nova-nas/tls" {
		t.Errorf("TLS.CertDir default=%q", cfg.TLS.CertDir)
	}
	if cfg.TLS.MinTLSVersion != "1.2" {
		t.Errorf("TLS.MinTLSVersion default=%q", cfg.TLS.MinTLSVersion)
	}
	if cfg.TLS.HTTPSAddr != "" {
		t.Errorf("TLS.HTTPSAddr default=%q want empty", cfg.TLS.HTTPSAddr)
	}
	if cfg.TLS.DisableHTTPRedirect {
		t.Error("TLS.DisableHTTPRedirect default should be false")
	}
}

func TestLoad_TLSEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("LISTEN_ADDR", ":8080")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("TLS_HTTPS_ADDR", ":8443")
	t.Setenv("TLS_HTTP_ADDR", ":8080")
	t.Setenv("TLS_CERT_PATH", "/srv/cert.pem")
	t.Setenv("TLS_KEY_PATH", "/srv/key.pem")
	t.Setenv("TLS_CERT_DIR", "/var/lib/nova/tls")
	t.Setenv("TLS_MIN_VERSION", "1.3")
	t.Setenv("TLS_DISABLE_HTTP_REDIRECT", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TLS.HTTPSAddr != ":8443" {
		t.Errorf("HTTPSAddr=%q", cfg.TLS.HTTPSAddr)
	}
	if cfg.TLS.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr=%q", cfg.TLS.HTTPAddr)
	}
	if cfg.TLS.CertPath != "/srv/cert.pem" {
		t.Errorf("CertPath=%q", cfg.TLS.CertPath)
	}
	if cfg.TLS.KeyPath != "/srv/key.pem" {
		t.Errorf("KeyPath=%q", cfg.TLS.KeyPath)
	}
	if cfg.TLS.CertDir != "/var/lib/nova/tls" {
		t.Errorf("CertDir=%q", cfg.TLS.CertDir)
	}
	if cfg.TLS.MinTLSVersion != "1.3" {
		t.Errorf("MinTLSVersion=%q", cfg.TLS.MinTLSVersion)
	}
	if !cfg.TLS.DisableHTTPRedirect {
		t.Error("DisableHTTPRedirect should be true")
	}
}

func TestLoad_AuthEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("LISTEN_ADDR", ":8080")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("OIDC_ISSUER_URL", "https://kc.example.com/realms/novanas")
	t.Setenv("OIDC_AUDIENCE", "nova-api")
	t.Setenv("OIDC_REQUIRED_ROLE_PREFIX", "nova-")
	t.Setenv("OIDC_CLIENT_ID", "nova-api-client")
	t.Setenv("OIDC_DISABLED", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Auth.IssuerURL != "https://kc.example.com/realms/novanas" {
		t.Errorf("IssuerURL=%q", cfg.Auth.IssuerURL)
	}
	if cfg.Auth.Audience != "nova-api" {
		t.Errorf("Audience=%q", cfg.Auth.Audience)
	}
	if cfg.Auth.RequiredRolePrefix != "nova-" {
		t.Errorf("RequiredRolePrefix=%q", cfg.Auth.RequiredRolePrefix)
	}
	if cfg.Auth.ClientID != "nova-api-client" {
		t.Errorf("ClientID=%q", cfg.Auth.ClientID)
	}
	if !cfg.Auth.Disabled {
		t.Error("Disabled should be true")
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
