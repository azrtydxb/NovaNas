# Policy for Loki to read secrets (if needed for S3 or other backends)
path "secret/data/observability/loki/*" {
  capabilities = ["read"]
}
path "secret/metadata/observability/loki/*" {
  capabilities = ["read"]
}
# Optional: PKI engine for issuing Loki TLS cert (if TLS is added)
path "pki_int/issue/loki" {
  capabilities = ["update"]
}
path "pki_int/sign/loki" {
  capabilities = ["update"]
}
