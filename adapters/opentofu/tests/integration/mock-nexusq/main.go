// Command mock-nexusq is a test-only NEXUS-Q API substitute.
// It uses a fixed AES-256-GCM key and must never be used outside E2E tests.
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync/atomic"
)

const testToken = "integration-test-token"

type request struct {
	KeyID     string `json:"key_id"`
	Plaintext string `json:"plaintext"`
	Envelope  string `json:"envelope"`
}
type response struct {
	Data string `json:"data"`
}

var encryptCalls atomic.Uint64
var decryptCalls atomic.Uint64

func main() {
	key := sha256.Sum256([]byte("NEXUS-Q Key Provider E2E fixture only; not a secret"))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		log.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]uint64{"encrypt_calls": encryptCalls.Load(), "decrypt_calls": decryptCalls.Load()})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "nexusq-mock-e2e")
	})
	mux.HandleFunc("/v1/encrypt", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		encryptCalls.Add(1)
		var in request
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in) != nil || in.KeyID == "" || in.Plaintext == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if _, err := base64.StdEncoding.DecodeString(in.Plaintext); err != nil {
			http.Error(w, "invalid plaintext", http.StatusBadRequest)
			return
		}
		nonce := make([]byte, gcm.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			http.Error(w, "random source failure", 500)
			return
		}
		sealed := gcm.Seal(nonce, nonce, []byte(in.Plaintext), nil)
		writeData(w, base64.StdEncoding.EncodeToString(sealed))
	})
	mux.HandleFunc("/v1/decrypt", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		decryptCalls.Add(1)
		var in request
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in) != nil || in.Envelope == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		sealed, err := base64.StdEncoding.DecodeString(in.Envelope)
		if err != nil || len(sealed) < gcm.NonceSize() {
			http.Error(w, "invalid envelope", http.StatusBadRequest)
			return
		}
		plain, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
		if err != nil {
			http.Error(w, "authentication failed", http.StatusBadRequest)
			return
		}
		// Provider decodes API data once, then decodes this Base64 DEK.
		writeData(w, base64.StdEncoding.EncodeToString(plain))
	})

	log.Printf("test-only mock listening on 127.0.0.1:18443")
	log.Fatal(http.ListenAndServe("127.0.0.1:18443", mux))
}

func authorized(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	if r.Header.Get("Authorization") != "Bearer "+testToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func writeData(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response{Data: data})
}
