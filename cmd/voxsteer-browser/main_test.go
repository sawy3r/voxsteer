package main

import (
	"net/http"
	"net/url"
	"testing"
)

func TestSessionTokenIsURLSafeAndRandom(t *testing.T) {
	first, err := sessionToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := sessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("session tokens unexpectedly match")
	}
	if len(first) != 43 || url.QueryEscape(first) != first {
		t.Fatalf("token is not an unpadded URL-safe 32-byte value: %q", first)
	}
}

func TestValidateLoopbackAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:47823", "127.0.0.1:0"} {
		if err := validateLoopbackAddress(address); err != nil {
			t.Fatalf("validateLoopbackAddress(%q): %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:47823", "[::1]:47823", "localhost:47823", "bad"} {
		if err := validateLoopbackAddress(address); err == nil {
			t.Fatalf("validateLoopbackAddress(%q) error = nil", address)
		}
	}
}

func TestSecureCloudClientRejectsInsecureRedirect(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "http://provider.example/transcribe", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := secureCloudClient().CheckRedirect(request, nil); err == nil {
		t.Fatal("CheckRedirect() error = nil, want insecure redirect error")
	}
}
