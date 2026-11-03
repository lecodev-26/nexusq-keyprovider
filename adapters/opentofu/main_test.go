package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		{"empty query rejected", "https://kms.example.test?", false},
		{"empty fragment rejected", "https://kms.example.test#", false},
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

func testEnv(serverURL string) map[string]string {
	return map[string]string{
		"NEXUSQ_URL": serverURL, "NEXUSQ_TOKEN": "test-token",
		"NEXUSQ_KEY_ID": "test-key", "NEXUSQ_CONTEXT_ID": "ctx-alpha",
	}
}

func encodeWrappedPayload(t *testing.T, contextID string, dek []byte) string {
	t.Helper()
	payload, err := json.Marshal(wrappedDEKPayload{
		Format: payloadFormat, Version: metadataVersionBound, ContextID: contextID, DEK: dek,
	})
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString(payload)))
}

func inspectEncryptPayload(t *testing.T, request map[string]string, wantContext string) {
	t.Helper()
	if request["key_id"] != "test-key" || request["metadata"] != envelopeMetadataV2 {
		t.Errorf("unexpected encrypt request: %#v", request)
	}
	encoded, err := base64.StdEncoding.DecodeString(request["plaintext"])
	if err != nil {
		t.Fatalf("decode plaintext wrapper: %v", err)
	}
	var payload wrappedDEKPayload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode wrapped payload: %v", err)
	}
	if payload.Format != payloadFormat || payload.Version != metadataVersionBound || payload.ContextID != wantContext || len(payload.DEK) != 32 {
		t.Errorf("unexpected wrapped payload: format=%q version=%d context=%q dek=%d", payload.Format, payload.Version, payload.ContextID, len(payload.DEK))
	}
}

func TestRunInitialKeyExchange(t *testing.T) {
	var encryptCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-token" || r.URL.Path != "/v1/encrypt" {
			t.Errorf("unexpected request: %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		inspectEncryptPayload(t, request, "ctx-alpha")
		encryptCalls++
		_ = json.NewEncoder(w).Encode(APIResponse{Data: base64.StdEncoding.EncodeToString([]byte("new-envelope"))})
	}))
	defer server.Close()

	var output bytes.Buffer
	env := testEnv(server.URL)
	if err := run(strings.NewReader("null"), &output, func(k string) string { return env[k] }); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("unexpected protocol output line count: %q", output.String())
	}
	var header struct {
		Magic   string `json:"magic"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("protocol header is not valid JSON: %v", err)
	}
	if header.Magic != "OpenTofu-External-Key-Provider" || header.Version != 1 {
		t.Fatalf("unexpected protocol header: %#v", header)
	}
	var result Result
	if err := json.Unmarshal([]byte(lines[1]), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Keys.EncryptionKey) != 32 || len(result.Keys.DecryptionKey) != 0 {
		t.Fatalf("unexpected keys: encryption=%d decryption=%d", len(result.Keys.EncryptionKey), len(result.Keys.DecryptionKey))
	}
	if result.Meta.ExternalData["format_version"] != float64(2) ||
		result.Meta.ExternalData["context_id"] != "ctx-alpha" ||
		result.Meta.ExternalData["envelope"] != "bmV3LWVudmVsb3Bl" || encryptCalls != 1 {
		t.Fatalf("unexpected metadata/call count: %#v calls=%d", result.Meta.ExternalData, encryptCalls)
	}
}

func TestRunRestoresPreviousKeyAndRotates(t *testing.T) {
	oldKey := bytes.Repeat([]byte{0x5a}, 32)
	outer := encodeWrappedPayload(t, "ctx-alpha", oldKey)
	var decryptCalls, encryptCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		switch r.URL.Path {
		case "/v1/decrypt":
			decryptCalls++
			if request["envelope"] != "old-envelope" {
				t.Errorf("decrypt envelope=%q", request["envelope"])
			}
			_ = json.NewEncoder(w).Encode(APIResponse{Data: outer})
		case "/v1/encrypt":
			inspectEncryptPayload(t, request, "ctx-alpha")
			encryptCalls++
			_ = json.NewEncoder(w).Encode(APIResponse{Data: base64.StdEncoding.EncodeToString([]byte("rotated-envelope"))})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	input := `{"external_data":{"format_version":2,"context_id":"ctx-alpha","envelope":"old-envelope","wrapping_key_id":"old-key"}}`
	var output bytes.Buffer
	env := testEnv(server.URL)
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
	if result.Meta.ExternalData["format_version"] != float64(2) || result.Meta.ExternalData["context_id"] != "ctx-alpha" {
		t.Fatalf("rotation did not preserve v2 context: %#v", result.Meta.ExternalData)
	}
}

func TestRunRejectsExternalContextMismatchBeforeAPI(t *testing.T) {
	var apiCalls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { apiCalls++ }))
	defer server.Close()
	input := `{"external_data":{"format_version":2,"context_id":"ctx-other","envelope":"old-envelope","wrapping_key_id":"old-key"}}`
	var output bytes.Buffer
	env := testEnv(server.URL)
	err := run(strings.NewReader(input), &output, func(k string) string { return env[k] })
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err=%v, want context mismatch", err)
	}
	if apiCalls != 0 || strings.Contains(output.String(), `"keys"`) {
		t.Fatalf("context mismatch reached API or emitted keys: calls=%d output=%q", apiCalls, output.String())
	}
}

func TestRunRejectsDecryptedPayloadContextMismatch(t *testing.T) {
	var encryptCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/encrypt" {
			encryptCalls++
			t.Error("must not wrap a fresh key after context mismatch")
			return
		}
		_ = json.NewEncoder(w).Encode(APIResponse{Data: encodeWrappedPayload(t, "ctx-other", bytes.Repeat([]byte{1}, 32))})
	}))
	defer server.Close()
	input := `{"external_data":{"format_version":2,"context_id":"ctx-alpha","envelope":"old-envelope","wrapping_key_id":"old-key"}}`
	var output bytes.Buffer
	env := testEnv(server.URL)
	err := run(strings.NewReader(input), &output, func(k string) string { return env[k] })
	if err == nil || !strings.Contains(err.Error(), "decrypted payload context_id") {
		t.Fatalf("err=%v, want decrypted payload context mismatch", err)
	}
	if encryptCalls != 0 || strings.Contains(output.String(), `"keys"`) {
		t.Fatalf("mismatch emitted keys or continued: encrypt=%d output=%q", encryptCalls, output.String())
	}
}

func TestRunRejectsLegacyMetadataByDefault(t *testing.T) {
	var apiCalls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { apiCalls++ }))
	defer server.Close()
	input := `{"external_data":{"format_version":1,"envelope":"old-envelope","wrapping_key_id":"old-key"}}`
	var output bytes.Buffer
	env := testEnv(server.URL)
	err := run(strings.NewReader(input), &output, func(k string) string { return env[k] })
	if err == nil || !strings.Contains(err.Error(), "legacy metadata v1 is disabled") {
		t.Fatalf("err=%v, want legacy migration opt-in error", err)
	}
	if apiCalls != 0 || strings.Contains(output.String(), `"keys"`) {
		t.Fatalf("legacy metadata reached API or emitted keys: calls=%d output=%q", apiCalls, output.String())
	}
}

func TestRunMigratesLegacyMetadataWhenExplicitlyEnabled(t *testing.T) {
	oldKey := bytes.Repeat([]byte{0x33}, 32)
	legacyInner := base64.StdEncoding.EncodeToString(oldKey)
	legacyOuter := base64.StdEncoding.EncodeToString([]byte(legacyInner))
	var decryptCalls, encryptCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/decrypt" {
			decryptCalls++
			_ = json.NewEncoder(w).Encode(APIResponse{Data: legacyOuter})
			return
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode encrypt request: %v", err)
		}
		inspectEncryptPayload(t, request, "ctx-alpha")
		encryptCalls++
		_ = json.NewEncoder(w).Encode(APIResponse{Data: base64.StdEncoding.EncodeToString([]byte("v2-envelope"))})
	}))
	defer server.Close()

	input := `{"external_data":{"format_version":1,"envelope":"legacy-envelope","wrapping_key_id":"old-key"}}`
	var output bytes.Buffer
	env := testEnv(server.URL)
	env["NEXUSQ_ALLOW_LEGACY_V1"] = "true"
	if err := run(strings.NewReader(input), &output, func(k string) string { return env[k] }); err != nil {
		t.Fatal(err)
	}
	var result Result
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if err := json.Unmarshal([]byte(lines[1]), &result); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Keys.DecryptionKey, oldKey) || result.Meta.ExternalData["format_version"] != float64(2) ||
		result.Meta.ExternalData["context_id"] != "ctx-alpha" || decryptCalls != 1 || encryptCalls != 1 {
		t.Fatalf("legacy migration failed: keys=%d metadata=%#v decrypt=%d encrypt=%d", len(result.Keys.DecryptionKey), result.Meta.ExternalData, decryptCalls, encryptCalls)
	}
}

func TestRunRequiresContextID(t *testing.T) {
	var output bytes.Buffer
	env := map[string]string{"NEXUSQ_URL": "https://kms.example.test", "NEXUSQ_TOKEN": "token", "NEXUSQ_KEY_ID": "key"}
	err := run(strings.NewReader("null"), &output, func(k string) string { return env[k] })
	if err == nil || !strings.Contains(err.Error(), "NEXUSQ_CONTEXT_ID") {
		t.Fatalf("err=%v, want required context ID", err)
	}
	if strings.Contains(output.String(), "\"keys\"") {
		t.Fatalf("provider emitted keys without context ID: %q", output.String())
	}
}

func TestRunRejectsInvalidLegacyMigrationFlag(t *testing.T) {
	var output bytes.Buffer
	env := testEnv("https://kms.example.test")
	env["NEXUSQ_ALLOW_LEGACY_V1"] = "yes"
	err := run(strings.NewReader("null"), &output, func(k string) string { return env[k] })
	if err == nil || !strings.Contains(err.Error(), "must be exactly true or false") {
		t.Fatalf("err=%v, want strict migration flag error", err)
	}
}

func TestParseProviderMetadataRejectsUnknownFields(t *testing.T) {
	meta := &Meta{ExternalData: map[string]any{
		"format_version": float64(2), "context_id": "ctx-alpha",
		"envelope": "envelope", "wrapping_key_id": "key", "unexpected": "value",
	}}
	if _, err := parseProviderMetadata(meta); err == nil {
		t.Fatal("expected unknown metadata field to be rejected")
	}
}

func TestValidateContextID(t *testing.T) {
	for _, value := range []string{"", " leading", "trailing ", "white space", "non-ascii-ñ", strings.Repeat("a", 129), "bad/slash"} {
		if err := validateContextID(value); err == nil {
			t.Errorf("validateContextID(%q) unexpectedly succeeded", value)
		}
	}
	for _, value := range []string{"ctx-alpha", "workspace_01", "a.b:c"} {
		if err := validateContextID(value); err != nil {
			t.Errorf("validateContextID(%q): %v", value, err)
		}
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

func TestReadMetadataAcceptsSingleJSONDocument(t *testing.T) {
	meta, err := readMetadata(strings.NewReader("  null  \n"))
	if err != nil {
		t.Fatal(err)
	}
	if meta != nil {
		t.Fatalf("metadata=%#v, want nil", meta)
	}
}

func TestReadMetadataRejectsTrailingOrMultipleJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "trailing garbage", input: `null trailing`},
		{name: "second JSON value", input: `null {}`},
		{name: "second scalar", input: `null true`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := readMetadata(strings.NewReader(tt.input)); err == nil {
				t.Fatalf("readMetadata(%q) unexpectedly succeeded", tt.input)
			}
		})
	}
}

func TestReadMetadataRejectsOversizedInput(t *testing.T) {
	input := strings.NewReader(`null` + strings.Repeat(" ", maxMetadataSize))
	if _, err := readMetadata(input); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err=%v, want metadata size error", err)
	}
}

func TestRunRejectsTrailingMetadataBeforeCallingAPI(t *testing.T) {
	var output bytes.Buffer
	env := map[string]string{
		"NEXUSQ_URL":    "https://kms.example.test",
		"NEXUSQ_TOKEN":  "test-token",
		"NEXUSQ_KEY_ID": "test-key",
	}
	err := run(strings.NewReader(`null {}`), &output, func(k string) string { return env[k] })
	if err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("err=%v, want multiple JSON values", err)
	}
	if strings.Contains(output.String(), `"keys"`) {
		t.Fatalf("provider emitted keys after invalid input: %q", output.String())
	}
}

func FuzzReadMetadata(f *testing.F) {
	for _, seed := range []string{
		"null",
		`{"external_data":{}}`,
		`{"external_data":{"format_version":1,"envelope":"YQ=="}}`,
		"",
		"null {}",
		`{"external_data":null}`,
		`{"external_data":{"format_version":1e100}}`,
		"[]",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		meta, err := readMetadata(strings.NewReader(input))
		if err != nil {
			return
		}
		encoded, err := json.Marshal(meta)
		if err != nil {
			t.Fatalf("marshal accepted metadata: %v", err)
		}
		if _, err := readMetadata(bytes.NewReader(encoded)); err != nil {
			t.Fatalf("accepted metadata did not round-trip: %v", err)
		}
	})
}

func FuzzValidateBaseURL(f *testing.F) {
	for _, seed := range []string{
		"https://kms.example.test",
		"http://127.0.0.1:8443",
		"http://kms.example.test",
		"https://kms.example.test?",
		"https://kms.example.test#",
		"https://user:pass@kms.example.test",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		base, err := validateBaseURL(raw)
		if err != nil {
			return
		}
		u, parseErr := url.Parse(base)
		if parseErr != nil {
			t.Fatalf("accepted URL cannot be parsed: %v", parseErr)
		}
		if u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") {
			t.Fatalf("accepted URL contains forbidden components: %q", raw)
		}
		ip := net.ParseIP(u.Hostname())
		loopbackHTTP := u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback()))
		if u.Scheme != "https" && !loopbackHTTP {
			t.Fatalf("accepted URL violates transport policy: %q", raw)
		}
	})
}

func FuzzDecodeAPIResponse(f *testing.F) {
	for _, seed := range []string{
		`{"data":"YQ==" }`,
		`{"data":""}`,
		`{"data":"%%%"}`,
		"null",
		"[]",
		"not-json",
		`{"data":"YQ==","extra":true}`,
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		data, err := decodeAPIResponse(input)
		if err != nil {
			return
		}
		if data == "" {
			t.Fatal("accepted empty API data")
		}
		if _, err := base64.StdEncoding.DecodeString(data); err != nil {
			t.Fatalf("accepted invalid Base64: %v", err)
		}
	})
}

func FuzzDecodeWrappedDEK(f *testing.F) {
	seed, _ := json.Marshal(wrappedDEKPayload{Format: payloadFormat, Version: metadataVersionBound, ContextID: "ctx-alpha", DEK: bytes.Repeat([]byte{7}, 32)})
	f.Add(base64.StdEncoding.EncodeToString([]byte(base64.StdEncoding.EncodeToString(seed))))
	for _, value := range []string{"", "%%%", "YQ==", "bnVsbA==", "e30="} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, outer string) {
		payload, err := decodeWrappedDEK(outer)
		if err != nil {
			return
		}
		defer clear(payload.DEK)
		if payload.Format != payloadFormat || payload.Version != metadataVersionBound || validateContextID(payload.ContextID) != nil || len(payload.DEK) != 32 {
			t.Fatalf("decoder accepted invalid payload: %#v", payload)
		}
	})
}

func FuzzParseProviderMetadata(f *testing.F) {
	for _, seed := range []string{
		"{\"external_data\":{\"format_version\":1,\"envelope\":\"e\",\"wrapping_key_id\":\"k\"}}",
		"{\"external_data\":{\"format_version\":2,\"context_id\":\"ctx\",\"envelope\":\"e\",\"wrapping_key_id\":\"k\"}}",
		"{\"external_data\":{}}", "null", "[]", "not-json",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		meta, err := readMetadata(strings.NewReader(raw))
		if err != nil || meta == nil {
			return
		}
		parsed, err := parseProviderMetadata(meta)
		if err != nil {
			return
		}
		if parsed.version != metadataVersionLegacy && parsed.version != metadataVersionBound {
			t.Fatalf("accepted version %d", parsed.version)
		}
		if parsed.envelope == "" || parsed.wrappingKey == "" {
			t.Fatal("accepted incomplete metadata")
		}
		if parsed.version == metadataVersionBound && validateContextID(parsed.contextID) != nil {
			t.Fatal("accepted invalid context")
		}
	})
}
