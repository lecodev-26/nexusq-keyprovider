package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Meta struct {
	ExternalData map[string]any `json:"external_data"`
}

type APIResponse struct {
	Data string `json:"data"`
}

type Result struct {
	Keys struct {
		EncryptionKey []byte `json:"encryption_key"`
		DecryptionKey []byte `json:"decryption_key,omitempty"`
	} `json:"keys"`
	Meta Meta `json:"meta"`
}

type providerMetadata struct {
	version     int
	envelope    string
	wrappingKey string
	contextID   string
}

type wrappedDEKPayload struct {
	Format    string `json:"format"`
	Version   int    `json:"version"`
	ContextID string `json:"context_id"`
	DEK       []byte `json:"dek"`
}

const (
	maxMetadataSize       = 1 << 20
	metadataVersionLegacy = 1
	metadataVersionBound  = 2
	payloadFormat         = "nexusq-opentofu-ekp"
	envelopeMetadataV2    = "opentofu-external-key-provider-v2"
)

func main() {
	if err := run(os.Stdin, os.Stdout, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "nexusq-keyprovider:", err)
		os.Exit(1)
	}
}

func run(input io.Reader, output io.Writer, getenv func(string) string) error {
	if _, err := fmt.Fprintln(output, `{"magic":"OpenTofu-External-Key-Provider","version":1}`); err != nil {
		return fmt.Errorf("write protocol header: %w", err)
	}
	meta, err := readMetadata(input)
	if err != nil {
		return err
	}
	base, err := validateBaseURL(getenv("NEXUSQ_URL"))
	if err != nil {
		return err
	}
	token, keyID := getenv("NEXUSQ_TOKEN"), getenv("NEXUSQ_KEY_ID")
	if token == "" || keyID == "" {
		return errors.New("NEXUSQ_TOKEN and NEXUSQ_KEY_ID are required")
	}
	contextID := getenv("NEXUSQ_CONTEXT_ID")
	if err := validateContextID(contextID); err != nil {
		return err
	}
	allowLegacy, err := parseLegacyMigrationFlag(getenv("NEXUSQ_ALLOW_LEGACY_V1"))
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	var old []byte
	defer func() { clear(old) }()
	if meta != nil {
		prior, err := parseProviderMetadata(meta)
		if err != nil {
			return err
		}
		switch prior.version {
		case metadataVersionLegacy:
			if !allowLegacy {
				return errors.New("legacy metadata v1 is disabled; set NEXUSQ_ALLOW_LEGACY_V1=true only for an intentional migration")
			}
		case metadataVersionBound:
			if prior.contextID != contextID {
				return errors.New("metadata context_id does not match NEXUSQ_CONTEXT_ID")
			}
		default:
			return errors.New("unsupported metadata version")
		}

		outer, err := callData(client, base, token, "/v1/decrypt", map[string]string{"envelope": prior.envelope})
		if err != nil {
			return fmt.Errorf("NEXUS-Q decrypt failed: %w", err)
		}
		if prior.version == metadataVersionLegacy {
			old, err = decodeLegacyDEK(outer)
		} else {
			var payload wrappedDEKPayload
			payload, err = decodeWrappedDEK(outer)
			if err == nil && payload.ContextID != contextID {
				clear(payload.DEK)
				err = errors.New("decrypted payload context_id does not match NEXUSQ_CONTEXT_ID")
			}
			if err == nil {
				old = payload.DEK
			}
		}
		if err != nil {
			return fmt.Errorf("recovered key invalid: %w", err)
		}
	}

	fresh := make([]byte, 32)
	defer clear(fresh)
	if _, err = rand.Read(fresh); err != nil {
		return fmt.Errorf("generate encryption key: %w", err)
	}
	wrapped, err := wrapDEK(client, base, token, keyID, contextID, fresh)
	if err != nil {
		return fmt.Errorf("NEXUS-Q encrypt failed: %w", err)
	}

	var out Result
	out.Keys.EncryptionKey = fresh
	if old != nil {
		out.Keys.DecryptionKey = old
	}
	out.Meta.ExternalData = map[string]any{
		"format_version":  metadataVersionBound,
		"context_id":      contextID,
		"envelope":        wrapped,
		"wrapping_key_id": keyID,
	}
	if err = json.NewEncoder(output).Encode(out); err != nil {
		return fmt.Errorf("encode provider response: %w", err)
	}
	return nil
}

func parseLegacyMigrationFlag(raw string) (bool, error) {
	switch raw {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, errors.New("NEXUSQ_ALLOW_LEGACY_V1 must be exactly true or false")
	}
}

func validateContextID(value string) error {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return errors.New("NEXUSQ_CONTEXT_ID is required and must be 1-128 ASCII characters")
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("._:-", r)) {
			return errors.New("NEXUSQ_CONTEXT_ID may contain only ASCII letters, digits, dot, underscore, colon, and hyphen")
		}
	}
	return nil
}

func parseProviderMetadata(meta *Meta) (providerMetadata, error) {
	if meta == nil || meta.ExternalData == nil {
		return providerMetadata{}, errors.New("metadata external_data missing")
	}
	ext := meta.ExternalData
	versionValue, ok := ext["format_version"].(float64)
	if !ok || versionValue != float64(int(versionValue)) {
		return providerMetadata{}, errors.New("unsupported metadata version")
	}
	result := providerMetadata{version: int(versionValue)}
	if result.version != metadataVersionLegacy && result.version != metadataVersionBound {
		return providerMetadata{}, errors.New("unsupported metadata version")
	}
	required := []string{"format_version", "envelope", "wrapping_key_id"}
	if result.version == metadataVersionBound {
		required = append(required, "context_id")
	}
	if len(ext) != len(required) {
		return providerMetadata{}, errors.New("metadata contains missing or unknown fields")
	}
	for _, key := range required {
		if _, exists := ext[key]; !exists {
			return providerMetadata{}, fmt.Errorf("metadata field %s missing", key)
		}
	}
	var valid bool
	result.envelope, valid = ext["envelope"].(string)
	if !valid || result.envelope == "" {
		return providerMetadata{}, errors.New("metadata envelope missing")
	}
	result.wrappingKey, valid = ext["wrapping_key_id"].(string)
	if !valid || result.wrappingKey == "" {
		return providerMetadata{}, errors.New("metadata wrapping_key_id missing")
	}
	if result.version == metadataVersionBound {
		result.contextID, valid = ext["context_id"].(string)
		if !valid || validateContextID(result.contextID) != nil {
			return providerMetadata{}, errors.New("metadata context_id invalid")
		}
	}
	return result, nil
}

func wrapDEK(c *http.Client, base, token, keyID, contextID string, dek []byte) (string, error) {
	if len(dek) != 32 {
		return "", errors.New("DEK must be exactly 32 bytes")
	}
	payload, err := json.Marshal(wrappedDEKPayload{
		Format: payloadFormat, Version: metadataVersionBound, ContextID: contextID, DEK: dek,
	})
	if err != nil {
		return "", fmt.Errorf("encode wrapped DEK payload: %w", err)
	}
	defer clear(payload)
	plaintext := base64.StdEncoding.EncodeToString(payload)
	return callData(c, base, token, "/v1/encrypt", map[string]string{
		"key_id": keyID, "plaintext": plaintext, "metadata": envelopeMetadataV2,
	})
}

func decodeLegacyDEK(outer string) ([]byte, error) {
	plain, err := base64.StdEncoding.DecodeString(outer)
	if err != nil {
		return nil, errors.New("NEXUS-Q decrypt response is invalid Base64")
	}
	defer clear(plain)
	dek, err := base64.StdEncoding.DecodeString(string(plain))
	if err != nil || len(dek) != 32 {
		clear(dek)
		return nil, errors.New("legacy DEK must decode to exactly 32 bytes")
	}
	return dek, nil
}

func decodeWrappedDEK(outer string) (wrappedDEKPayload, error) {
	encodedPayload, err := base64.StdEncoding.DecodeString(outer)
	if err != nil {
		return wrappedDEKPayload{}, errors.New("NEXUS-Q decrypt response is invalid Base64")
	}
	defer clear(encodedPayload)
	payloadBytes, err := base64.StdEncoding.DecodeString(string(encodedPayload))
	if err != nil {
		return wrappedDEKPayload{}, errors.New("decrypted payload is invalid Base64")
	}
	defer clear(payloadBytes)

	decoder := json.NewDecoder(bytes.NewReader(payloadBytes))
	decoder.DisallowUnknownFields()
	var payload wrappedDEKPayload
	if err := decoder.Decode(&payload); err != nil {
		return wrappedDEKPayload{}, fmt.Errorf("decode wrapped DEK payload: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		clear(payload.DEK)
		if err == nil {
			return wrappedDEKPayload{}, errors.New("wrapped DEK payload contains multiple JSON values")
		}
		return wrappedDEKPayload{}, fmt.Errorf("wrapped DEK payload has trailing data: %w", err)
	}
	if payload.Format != payloadFormat || payload.Version != metadataVersionBound {
		clear(payload.DEK)
		return wrappedDEKPayload{}, errors.New("unsupported wrapped DEK payload format or version")
	}
	if err := validateContextID(payload.ContextID); err != nil {
		clear(payload.DEK)
		return wrappedDEKPayload{}, errors.New("wrapped DEK payload context_id invalid")
	}
	if len(payload.DEK) != 32 {
		clear(payload.DEK)
		return wrappedDEKPayload{}, errors.New("wrapped DEK payload must contain a 32-byte DEK")
	}
	return payload, nil
}

func readMetadata(input io.Reader) (*Meta, error) {
	data, err := io.ReadAll(io.LimitReader(input, maxMetadataSize+1))
	if err != nil {
		return nil, fmt.Errorf("read OpenTofu metadata: %w", err)
	}
	if len(data) > maxMetadataSize {
		return nil, fmt.Errorf("OpenTofu metadata exceeds %d-byte limit", maxMetadataSize)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	var meta *Meta
	if err := decoder.Decode(&meta); err != nil {
		return nil, fmt.Errorf("decode OpenTofu metadata: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("decode OpenTofu metadata: multiple JSON values")
		}
		return nil, fmt.Errorf("decode OpenTofu metadata: trailing data: %w", err)
	}
	return meta, nil
}

func validateBaseURL(raw string) (string, error) {
	base := strings.TrimRight(raw, "/")
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return "", errors.New("invalid NEXUSQ_URL")
	}
	// Reject even empty query/fragment delimiters: concatenating API paths to
	// such URLs can turn the path into a query or fragment instead of a route.
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") {
		return "", errors.New("NEXUSQ_URL must not contain userinfo, query or fragment")
	}
	ip := net.ParseIP(u.Hostname())
	loopbackHTTP := u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback()))
	if u.Scheme != "https" && !loopbackHTTP {
		return "", errors.New("HTTP is allowed only on loopback; remote endpoints require HTTPS")
	}
	return base, nil
}

func callData(c *http.Client, base, token, path string, payload any) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	defer clear(b)
	req, err := http.NewRequest("POST", base+path, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return "", err
	}
	if len(data) > 1<<20 {
		return "", errors.New("response too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return decodeAPIResponse(data)
}

func decodeAPIResponse(data []byte) (string, error) {
	var r APIResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return "", err
	}
	if r.Data == "" {
		return "", errors.New("NEXUS-Q returned empty data")
	}
	if _, err := base64.StdEncoding.DecodeString(r.Data); err != nil {
		return "", errors.New("NEXUS-Q returned invalid Base64 data")
	}
	return r.Data, nil
}
