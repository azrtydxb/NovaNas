# Policy for Grafana to read secrets (SMTP, OAuth, etc.)
path "secret/data/observability/grafana/*" {
  capabilities = ["read"]
}
path "secret/metadata/observability/grafana/*" {
  capabilities = ["read"]
}
# Optional: PKI engine for issuing Grafana TLS cert
path "pki_int/issue/grafana" {
  capabilities = ["update"]
}
path "pki_int/sign/grafana" {
  capabilities = ["update"]
}
