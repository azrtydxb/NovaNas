# Policy for Prometheus to read secrets for targets requiring auth
path "secret/data/observability/prometheus/*" {
  capabilities = ["read"]
}
path "secret/metadata/observability/prometheus/*" {
  capabilities = ["read"]
}
# Optional: PKI engine for issuing Prometheus TLS cert (if using OpenBao PKI)
path "pki_int/issue/prometheus" {
  capabilities = ["update"]
}
path "pki_int/sign/prometheus" {
  capabilities = ["update"]
}
