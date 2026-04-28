package middleware

import "regexp"

// secretPatterns matches `"key":"value"` or `"key":value` shapes for
// common credential field names. Replacement preserves the key and
// substitutes the value with "<redacted>".
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)("?password"?\s*[:=]\s*)("[^"]*"|[^,\s}]+)`),
	regexp.MustCompile(`(?i)("?passphrase"?\s*[:=]\s*)("[^"]*"|[^,\s}]+)`),
	regexp.MustCompile(`(?i)("?secret"?\s*[:=]\s*)("[^"]*"|[^,\s}]+)`),
	regexp.MustCompile(`(?i)("?token"?\s*[:=]\s*)("[^"]*"|[^,\s}]+)`),
}

// RedactSecrets replaces values for password/passphrase/secret/token
// keys with the literal string "<redacted>". Operates on raw bytes so
// it can be applied to a JSON body before persisting to audit_log.
func RedactSecrets(s []byte) []byte {
	out := s
	for _, re := range secretPatterns {
		out = re.ReplaceAll(out, []byte(`$1"<redacted>"`))
	}
	return out
}
