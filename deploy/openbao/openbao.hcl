ui = true

storage "raft" {
  path    = "/var/lib/openbao"
  node_id = "node-novanas"
}

listener "tcp" {
  address       = "0.0.0.0:8200"
  tls_cert_file = "/etc/openbao/tls/cert.pem"
  tls_key_file  = "/etc/openbao/tls/key.pem"
}

# TPM-backed auto-unseal via PKCS#11 when supported by OpenBao OSS.
# Until then, unseal keys are decrypted at boot by nova-bao-unseal service.
# seal "pkcs11" {
#   lib            = "/usr/lib/x86_64-linux-gnu/pkcs11/libtpm2_pkcs11.so.1"
#   slot           = "0"
#   pin            = "file:///etc/openbao/pkcs11.pin"
#   key_label      = "openbao-unseal"
#   hmac_key_label = "openbao-hmac"
# }

api_addr = "https://localhost:8200"
cluster_addr = "https://localhost:8201"

disable_mlock = false
