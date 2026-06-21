// Package cred derives a keyed hash of a caller credential, used solely to bind
// cache handles so a handle can be redeemed only by the same credential.
package cred

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// HMAC returns the hex HMAC-SHA256 of credential keyed by secret. The credential
// itself is never stored or logged; only this hash is.
func HMAC(secret, credential string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(credential))
	return hex.EncodeToString(mac.Sum(nil))
}
