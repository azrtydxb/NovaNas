# Policy for Alertmanager to read SMTP and notification secrets
path "secret/data/observability/alertmanager/*" {
  capabilities = ["read"]
}
path "secret/metadata/observability/alertmanager/*" {
  capabilities = ["read"]
}
# Optional: PKI engine for issuing Alertmanager TLS cert
path "pki_int/issue/alertmanager" {
  capabilities = ["update"]
}
path "pki_int/sign/alertmanager" {
  capabilities = ["update"]
}
