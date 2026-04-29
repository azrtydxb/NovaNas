# Policy for the Keycloak service account to read its DB password and
# realm config from OpenBao.
path "secret/data/keycloak/*" {
  capabilities = ["read"]
}
path "secret/metadata/keycloak/*" {
  capabilities = ["read"]
}
