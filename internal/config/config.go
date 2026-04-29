// Package config loads application config from environment variables.
package config

import (
	"errors"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
	ListenAddr  string `envconfig:"LISTEN_ADDR" required:"true"`
	RedisURL    string `envconfig:"REDIS_URL" required:"true"`
	ZFSBin      string `envconfig:"ZFS_BIN" default:"/sbin/zfs"`
	ZpoolBin    string `envconfig:"ZPOOL_BIN" default:"/sbin/zpool"`
	LsblkBin    string `envconfig:"LSBLK_BIN" default:"/usr/bin/lsblk"`
	LogLevel    string `envconfig:"LOG_LEVEL" default:"info"`

	// NFSRequireKerberos, when true, makes the NFS exports manager prepend
	// `sec=krb5p` to every export's client options. Operators with no KDC
	// or no host keytab should leave this false (the default) — exports
	// will use sec=sys and behave as before.
	NFSRequireKerberos bool `envconfig:"NFS_REQUIRE_KERBEROS" default:"false"`

	// Krb5KDCEnabled exposes the embedded MIT KDC's principal-management
	// API endpoints (POST/GET/DELETE /api/v1/krb5/principals + KDC status).
	// Operators with no embedded KDC (BYO Active Directory or FreeIPA)
	// should leave this false. Defaults false to preserve the original
	// "no KDC" architecture for upgrades.
	Krb5KDCEnabled bool `envconfig:"KRB5_KDC_ENABLED" default:"false"`

	// Krb5Realm overrides the realm name used by the embedded KDC.
	// Empty falls back to the krb5 package default (NOVANAS.LOCAL).
	Krb5Realm string `envconfig:"KRB5_REALM"`

	// MetricsAddr, when set, binds the Prometheus /metrics endpoint to a
	// separate listener (e.g. ":9100") so the public API listener does
	// not expose it. Empty (the default) keeps /metrics on the main
	// listener at the root path. The endpoint is always public — operators
	// must use METRICS_ADDR or network-level controls to restrict access.
	MetricsAddr string `envconfig:"METRICS_ADDR"`

	TLS  TLSConfig
	Auth AuthConfig
	SMTP SMTPConfig

	// AlertmanagerURL is the upstream Alertmanager API base URL used by
	// the /api/v1/alerts* pass-through endpoints. Defaults to the
	// loopback-bound AM that ships with the appliance.
	AlertmanagerURL string `envconfig:"ALERTMANAGER_URL" default:"http://127.0.0.1:9093"`

	// LokiURL is the upstream Loki API base URL used by the
	// /api/v1/logs* pass-through endpoints.
	LokiURL string `envconfig:"LOKI_URL" default:"http://127.0.0.1:3100"`

	// Keycloak admin API. KeycloakAdminURL defaults to deriving from
	// OIDC_ISSUER_URL (issuer is .../realms/<realm>; admin is
	// .../admin/realms/<realm>). KeycloakAdminClientID +
	// KeycloakAdminClientSecretFile authenticate via client_credentials.
	// Empty client id/secret disables the /auth/sessions and
	// /auth/login-history endpoints.
	KeycloakAdminURL              string `envconfig:"KEYCLOAK_ADMIN_URL"`
	KeycloakAdminClientID         string `envconfig:"KEYCLOAK_ADMIN_CLIENT_ID"`
	KeycloakAdminClientSecretFile string `envconfig:"KEYCLOAK_ADMIN_CLIENT_SECRET_FILE"`

	// Tier 2 plugin engine. MarketplaceIndexURL points at index.json
	// for the NovaNAS marketplace; MarketplaceTrustKeyPath is the
	// cosign public key used to verify package signatures. Empty
	// MarketplaceTrustKeyPath disables signature verification — DO
	// NOT do this in production; it disables the entire trust chain.
	// MarketplaceCosignBin, when set and non-empty, switches the
	// verifier from native-Go PEM verification to shelling out to
	// `cosign verify-blob` (operators who need rekor / transparency).
	MarketplaceIndexURL    string `envconfig:"MARKETPLACE_INDEX_URL" default:"https://raw.githubusercontent.com/azrtydxb/NovaNas-packages/main/index.json"`
	MarketplaceTrustKeyPath string `envconfig:"MARKETPLACE_TRUST_KEY_PATH" default:"/etc/nova-nas/trust/marketplace.pub"`
	MarketplaceCosignBin    string `envconfig:"MARKETPLACE_COSIGN_BIN"`
	// PluginsRoot is the on-disk directory where unpacked plugin trees
	// (UI bundles, manifest.yaml, etc.) live. Default
	// /var/lib/nova-nas/plugins.
	PluginsRoot string `envconfig:"PLUGINS_ROOT" default:"/var/lib/nova-nas/plugins"`

	// PluginsCACertPath / PluginsCAKeyPath point at the local NovaNAS
	// CA used by the plugin engine to mint server certs for plugins
	// that claim a `tlsCert` need. Defaults match the host bootstrap
	// (see deploy/observability/issue-certs.sh).
	PluginsCACertPath string `envconfig:"PLUGINS_CA_CERT_PATH" default:"/etc/nova-ca/ca.crt"`
	PluginsCAKeyPath  string `envconfig:"PLUGINS_CA_KEY_PATH" default:"/etc/nova-ca/ca.key"`

	// PluginsSystemctlBin overrides the systemctl binary used by the
	// plugin systemd deployer. Empty falls back to /bin/systemctl.
	PluginsSystemctlBin string `envconfig:"PLUGINS_SYSTEMCTL_BIN"`
}

// SMTPConfig configures the outbound SMTP relay used by transactional
// email (password reset, invite, weekly summary) and synchronous test
// sends from the API.
//
// Host empty disables outbound email; PUT /api/v1/notifications/smtp
// can populate it later at runtime without a restart. The password is
// loaded from a file (PasswordFile) rather than directly from the env
// to avoid leaking it via /proc/<pid>/environ.
type SMTPConfig struct {
	Host         string `envconfig:"SMTP_HOST"`
	Port         int    `envconfig:"SMTP_PORT" default:"587"`
	Username     string `envconfig:"SMTP_USERNAME"`
	PasswordFile string `envconfig:"SMTP_PASSWORD_FILE"`
	From         string `envconfig:"SMTP_FROM"`
	// TLSMode is one of "none", "starttls" (default), "tls".
	TLSMode      string `envconfig:"SMTP_TLS_MODE" default:"starttls"`
	MaxPerMinute int    `envconfig:"SMTP_MAX_PER_MINUTE" default:"30"`
}

// AuthConfig configures OIDC token verification for the HTTP API.
//
// When Disabled is true, the API skips both verification and per-route
// permission enforcement and logs a loud WARN line at startup. Intended
// for local development only.
type AuthConfig struct {
	// IssuerURL is Keycloak's realm URL (e.g.
	// "https://kc.example.com/realms/novanas"). Required unless Disabled.
	IssuerURL string `envconfig:"OIDC_ISSUER_URL"`

	// Audience is the expected `aud` claim. Required unless Disabled.
	Audience string `envconfig:"OIDC_AUDIENCE"`

	// RequiredRolePrefix optionally filters realm/resource roles. Empty
	// means accept all roles.
	RequiredRolePrefix string `envconfig:"OIDC_REQUIRED_ROLE_PREFIX"`

	// ClientID is the Keycloak client whose resource_access.<client>.roles
	// should be merged into the Identity. Empty means realm roles only.
	ClientID string `envconfig:"OIDC_CLIENT_ID"`

	// Disabled bypasses authentication entirely. Dev only.
	Disabled bool `envconfig:"OIDC_DISABLED"`
}

// TLSConfig configures the HTTPS listener and optional HTTP redirect.
//
// HTTPSAddr empty disables TLS entirely (legacy plain-HTTP mode via
// Config.ListenAddr). When HTTPSAddr is set and CertPath/KeyPath are
// both empty, a self-signed cert is generated under CertDir at first
// boot.
type TLSConfig struct {
	// Listen address for HTTPS (e.g. ":8443"). Empty disables HTTPS.
	HTTPSAddr string `envconfig:"TLS_HTTPS_ADDR"`

	// Listen address for HTTP redirect (e.g. ":8080"). Empty disables.
	HTTPAddr string `envconfig:"TLS_HTTP_ADDR"`

	// CertPath / KeyPath: operator-supplied PEM files. If both empty
	// AND HTTPSAddr is set, a self-signed cert is generated at
	// <CertDir>/cert.pem + key.pem on first boot.
	CertPath string `envconfig:"TLS_CERT_PATH"`
	KeyPath  string `envconfig:"TLS_KEY_PATH"`

	// CertDir is where self-signed and rotated certs live when no
	// operator paths are given. Default /etc/nova-nas/tls.
	CertDir string `envconfig:"TLS_CERT_DIR" default:"/etc/nova-nas/tls"`

	// MinTLSVersion: "1.2" or "1.3". Default "1.2".
	MinTLSVersion string `envconfig:"TLS_MIN_VERSION" default:"1.2"`

	// CipherSuites optional override; empty = Go default modern set.
	CipherSuites []string `envconfig:"-"`

	// SelfSignedHostname: name to put in the cert CN/SAN when
	// generating self-signed. Default os.Hostname().
	SelfSignedHostname string `envconfig:"-"`

	// DisableHTTPRedirect: if true, the HTTP listener serves nothing
	// (or doesn't bind at all). Default false (redirect enabled).
	DisableHTTPRedirect bool `envconfig:"TLS_DISABLE_HTTP_REDIRECT"`
}

func Load() (*Config, error) {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return nil, err
	}
	if c.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	if c.ListenAddr == "" {
		return nil, errors.New("LISTEN_ADDR is required")
	}
	if c.RedisURL == "" {
		return nil, errors.New("REDIS_URL is required")
	}
	return &c, nil
}
