package cred

import "testing"

func TestHMACStableAndKeyed(t *testing.T) {
	a := HMAC("secret", "tok")
	if a == "" {
		t.Fatalf("empty hmac")
	}
	if a != HMAC("secret", "tok") {
		t.Fatalf("HMAC must be deterministic")
	}
	if a == HMAC("other-secret", "tok") {
		t.Fatalf("HMAC must depend on the secret")
	}
	if a == HMAC("secret", "different") {
		t.Fatalf("HMAC must depend on the credential")
	}
}

func TestHMACEmptyCredential(t *testing.T) {
	if HMAC("secret", "") != HMAC("secret", "") {
		t.Fatalf("empty-credential HMAC must be stable")
	}
}
