package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testHTTPClient() *http.Client {
	return &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func TestValidateBaseURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		ok   bool
	}{
		{"https remote", "https://kms.example.test", true},
		{"http localhost", "http://localhost:18443", true},
		{"http ipv4 loopback", "http://127.0.0.1:18443", true},
		{"http ipv6 loopback", "http://[::1]:18443", true},
		{"http remote rejected", "http://kms.example.test", false},
		{"ftp rejected", "ftp://localhost", false},
		{"missing host", "https://", false},
		{"userinfo rejected", "https://user:pass@kms.example.test", false},
		{"query rejected", "https://kms.example.test?token=x", false},
		{"fragment rejected", "https://kms.example.test#frag", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateBaseURL(tt.raw)
			if (err == nil) != tt.ok {
				t.Fatalf("validateBaseURL(%q) err=%v, want success=%v", tt.raw, err, tt.ok)
			}
			if tt.ok && got == "" {
				t.Fatal("expected normalized URL")
			}
		})
	}
}

func TestCallDataSendsAuthenticatedJSONPost(t *testing.T) {
	want := base64.StdEncoding.EncodeToString([]byte("envelope-value"))
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/encrypt" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("authorization=%q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":%q}`, want)
	}))
	defer s.Close()

	got, err := callData(testHTTPClient(), s.URL, "test-token", "/v1/encrypt", map[string]string{"key_id": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("data=%q, want %q", got, want)
	}
}

func TestCallDataRejectsHTTPError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not authorized", http.StatusUnauthorized)
	}))
	defer s.Close()
	_, err := callData(testHTTPClient(), s.URL, "bad-token", "/v1/decrypt", map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("err=%v, want HTTP 401", err)
	}
}

func TestCallDataRejectsInvalidJSON(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer s.Close()
	if _, err := callData(testHTTPClient(), s.URL, "token", "/v1/encrypt", nil); err == nil {
		t.Fatal("expected JSON decoding error")
	}
}

func TestCallDataRejectsInvalidBase64(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":"%%%"}`))
	}))
	defer s.Close()
	_, err := callData(testHTTPClient(), s.URL, "token", "/v1/encrypt", nil)
	if err == nil || !strings.Contains(err.Error(), "invalid Base64") {
		t.Fatalf("err=%v, want invalid Base64", err)
	}
}

func TestCallDataRejectsOversizedResponse(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", (1<<20)+1)))
	}))
	defer s.Close()
	_, err := callData(testHTTPClient(), s.URL, "token", "/v1/encrypt", nil)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err=%v, want too large", err)
	}
}

func TestCallDataRejectsRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":"YQ=="}`))
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	_, err := callData(testHTTPClient(), redirect.URL, "token", "/v1/encrypt", nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 302") {
		t.Fatalf("err=%v, want HTTP 302", err)
	}
}

func TestCallDataRejectsEmptyData(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":""}`))
	}))
	defer s.Close()
	_, err := callData(testHTTPClient(), s.URL, "token", "/v1/encrypt", nil)
	if err == nil || !strings.Contains(err.Error(), "empty data") {
		t.Fatalf("err=%v, want empty data", err)
	}
}

func TestRunInitialKeyExchange(t *testing.T) {
	var encryptCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected request: %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/v1/encrypt" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload["key_id"] != "test-key" || payload["metadata"] != "opentofu-external-key-provider-v1" {
			t.Errorf("unexpected encrypt payload: %#v", payload)
		}
		key, err := base64.StdEncoding.DecodeString(payload["plaintext"])
		if err != nil || len(key) != 32 {
			t.Errorf("plaintext is not a 32-byte DEK: len=%d err=%v", len(key), err)
		}
		encryptCalls++
		_ = json.NewEncoder(w).Encode(APIResponse{Data: base64.StdEncoding.EncodeToString([]byte("new-envelope"))})
	}))
	defer server.Close()

	var output bytes.Buffer
	env := map[string]string{"NEXUSQ_URL": server.URL, "NEXUSQ_TOKEN": "test-token", "NEXUSQ_KEY_ID": "test-key"}
	if err := run(strings.NewReader("null"), &output, func(k string) string { return env[k] }); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "OpenTofu-External-Key-Provider") {
		t.Fatalf("unexpected protocol output: %q", output.String())
	}
	var result Result
	if err := json.Unmarshal([]byte(lines[1]), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Keys.EncryptionKey) != 32 || len(result.Keys.DecryptionKey) != 0 {
		t.Fatalf("unexpected keys: encryption=%d decryption=%d", len(result.Keys.EncryptionKey), len(result.Keys.DecryptionKey))
	}
	if result.Meta.ExternalData["envelope"] != "bmV3LWVudmVsb3Bl" || encryptCalls != 1 {
		t.Fatalf("unexpected metadata/call count: %#v calls=%d", result.Meta.ExternalData, encryptCalls)
	}
}

func TestRunRestoresPreviousKeyAndRotates(t *testing.T) {
	oldKey := bytes.Repeat([]byte{0x5a}, 32)
	inner := base64.StdEncoding.EncodeToString(oldKey)
	outer := base64.StdEncoding.EncodeToString([]byte(inner))
	var decryptCalls, encryptCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		switch r.URL.Path {
		case "/v1/decrypt":
			decryptCalls++
			if payload["envelope"] != "old-envelope" {
				t.Errorf("decrypt envelope=%q", payload["envelope"])
			}
			_ = json.NewEncoder(w).Encode(APIResponse{Data: outer})
		case "/v1/encrypt":
			encryptCalls++
			fresh, err := base64.StdEncoding.DecodeString(payload["plaintext"])
			if err != nil || len(fresh) != 32 {
				t.Errorf("new key invalid: len=%d err=%v", len(fresh), err)
			}
			_ = json.NewEncoder(w).Encode(APIResponse{Data: base64.StdEncoding.EncodeToString([]byte("rotated-envelope"))})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	input := `{"external_data":{"format_version":1,"envelope":"old-envelope"}}`
	var output bytes.Buffer
	env := map[string]string{"NEXUSQ_URL": server.URL, "NEXUSQ_TOKEN": "test-token", "NEXUSQ_KEY_ID": "new-key"}
	if err := run(strings.NewReader(input), &output, func(k string) string { return env[k] }); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	var result Result
	if err := json.Unmarshal([]byte(lines[1]), &result); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Keys.DecryptionKey, oldKey) {
		t.Fatal("previous decryption key was not restored exactly")
	}
	if len(result.Keys.EncryptionKey) != 32 || bytes.Equal(result.Keys.EncryptionKey, oldKey) {
		t.Fatal("rotation did not produce a fresh 32-byte key")
	}
	if decryptCalls != 1 || encryptCalls != 1 {
		t.Fatalf("decrypt calls=%d encrypt calls=%d", decryptCalls, encryptCalls)
	}
	if result.Meta.ExternalData["envelope"] != "cm90YXRlZC1lbnZlbG9wZQ==" {
		t.Fatalf("unexpected rotated envelope: %#v", result.Meta.ExternalData)
	}
}

func TestRunFailsClosedBeforeReturningKeys(t *testing.T) {
	var output bytes.Buffer
	err := run(strings.NewReader("null"), &output, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "NEXUSQ_URL") {
		t.Fatalf("err=%v, want invalid NEXUSQ_URL", err)
	}
	if strings.Contains(output.String(), `"keys"`) {
		t.Fatalf("provider emitted keys after configuration failure: %q", output.String())
	}
}
