package liveattestation

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// MaxHelperConcurrency bounds -max-concurrent: each slot is one request that
// may wait on DeviceCheck, so the bound also bounds goroutines and fallbacks.
const MaxHelperConcurrency = 64

// NewHelperHandler exposes only local capability checks and fresh generation.
// It never accepts caller-supplied signals, account credentials, or commands.
// It serves one generation at a time (the original behaviour).
func NewHelperHandler(provider Provider, token string) (http.Handler, error) {
	return NewHelperHandlerWithLimit(provider, token, 1)
}

// NewHelperHandlerWithLimit is NewHelperHandler with up to maxConcurrent
// generations in flight; a request beyond that is still answered 429 at once.
// Every request still gets its own freshly generated proof.
func NewHelperHandlerWithLimit(provider Provider, token string, maxConcurrent int) (http.Handler, error) {
	if provider == nil || !validHelperToken(token) {
		return nil, errors.New("helper requires a provider and a strong token")
	}
	if maxConcurrent < 1 || maxConcurrent > MaxHelperConcurrency {
		return nil, errors.New("helper concurrency must be between 1 and 64")
	}
	expected := sha256.Sum256([]byte("Bearer " + token))
	busy := make(chan struct{}, maxConcurrent)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		actual := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		method := ""
		switch r.URL.Path {
		case "/health":
			method = http.MethodGet
		case "/v1/attestation":
			method = http.MethodPost
		default:
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		if r.Method != method {
			w.Header().Set("Allow", method)
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		// Even a chunked body is rejected without reading unbounded input.
		body, err := io.ReadAll(io.LimitReader(r.Body, 1))
		if err != nil || len(body) != 0 || r.URL.RawQuery != "" {
			http.Error(w, `{"error":"request payloads are not accepted"}`, http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
		defer cancel()
		if r.URL.Path == "/health" {
			if provider.Check(ctx) != nil {
				http.Error(w, `{"error":"local runtime unavailable"}`, http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(remoteHelperResponse{Supported: true})
			return
		}
		select {
		case busy <- struct{}{}:
			defer func() { <-busy }()
		default:
			http.Error(w, `{"error":"helper busy"}`, http.StatusTooManyRequests)
			return
		}
		header, err := provider.Generate(ctx)
		if err != nil || validateRemoteHeader(header) != nil {
			http.Error(w, `{"error":"local attestation generation unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(remoteHelperResponse{Attestation: header})
	}), nil
}
