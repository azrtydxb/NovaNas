# Policy for the NovaNAS API service account to read secrets
# and issue TLS certificates via PKI engine.
path "secret/data/novanas/*" {
  capabilities = ["read"]
}
path "secret/metadata/novanas/*" {
  capabilities = ["read"]
}
# PKI engine for issuing the API's TLS cert.
path "pki_int/issue/novanas-api" {
  capabilities = ["update"]
}
path "pki_int/sign/novanas-api" {
  capabilities = ["update"]
}
