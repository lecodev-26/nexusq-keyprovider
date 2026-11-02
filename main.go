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

func main() {
	if err := run(os.Stdin, os.Stdout, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "nexusq-keyprovider:", err)
		os.Exit(1)
	}
}

const maxMetadataSize = 1 << 20

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
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	var old []byte
	if meta != nil {
		if meta.ExternalData == nil {
			return errors.New("metadata external_data missing")
		}
		v, ok := meta.ExternalData["format_version"].(float64)
		if !ok || v != 1 {
			return errors.New("unsupported metadata version")
		}
		envelope, ok := meta.ExternalData["envelope"].(string)
		if !ok || envelope == "" {
			return errors.New("metadata envelope missing")
		}
		outer, err := callData(client, base, token, "/v1/decrypt", map[string]string{"envelope": envelope})
		if err != nil {
			return fmt.Errorf("NEXUS-Q decrypt failed: %w", err)
		}
		plain, err := base64.StdEncoding.DecodeString(outer)
		if err != nil {
			return errors.New("NEXUS-Q decrypt response is invalid")
		}
		old, err = base64.StdEncoding.DecodeString(string(plain))
		if err != nil || len(old) != 32 {
			return errors.New("recovered key invalid")
		}
	}

	fresh := make([]byte, 32)
	if _, err = rand.Read(fresh); err != nil {
		return fmt.Errorf("generate encryption key: %w", err)
	}
	wrapped, err := callData(client, base, token, "/v1/encrypt", map[string]string{
		"key_id": keyID, "plaintext": base64.StdEncoding.EncodeToString(fresh), "metadata": "opentofu-external-key-provider-v1",
	})
	if err != nil {
		return fmt.Errorf("NEXUS-Q encrypt failed: %w", err)
	}
	var out Result
	out.Keys.EncryptionKey = fresh
	if old != nil {
		out.Keys.DecryptionKey = old
	}
	out.Meta.ExternalData = map[string]any{"format_version": 1, "envelope": wrapped, "wrapping_key_id": keyID}
	if err = json.NewEncoder(output).Encode(out); err != nil {
		return fmt.Errorf("encode provider response: %w", err)
	}
	return nil
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
