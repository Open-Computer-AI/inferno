package liveattestation

import (
	"bytes"
	"context"
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

const maxHelperResponse = 20 * 1024

// NewProvider preserves native behavior unless a remote helper is explicitly
// configured. Invalid remote configuration never silently falls back locally.
func NewProvider() Provider {
	endpoint := strings.TrimSpace(os.Getenv("LIVE_ATTESTATION_URL"))
	tokenFile := strings.TrimSpace(os.Getenv("LIVE_ATTESTATION_TOKEN_FILE"))
	if endpoint == "" && tokenFile == "" {
		return newLocalProvider()
	}
	token, err := ReadHelperTokenFile(tokenFile)
	if err != nil {
		return unavailableProvider{err}
	}
	provider, err := NewRemoteProvider(endpoint, token)
	if err != nil {
		return unavailableProvider{err}
	}
	return provider
}

// NewLocalProvider is for the Mac helper, which must never call itself through
// remote-provider configuration inherited from the gateway environment.
func NewLocalProvider() Provider { return newLocalProvider() }

type unavailableProvider struct{ err error }

func (p unavailableProvider) Check(context.Context) error              { return p.err }
func (p unavailableProvider) Generate(context.Context) (string, error) { return "", p.err }

type remoteProvider struct {
	endpoint string
	token    string
	client   *http.Client
}

type remoteHelperResponse struct {
	Supported   bool   `json:"supported,omitempty"`
	Attestation string `json:"attestation,omitempty"`
}

// NewRemoteProvider accepts HTTPS or local-development HTTP origins only.
func NewRemoteProvider(endpoint, token string) (Provider, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("remote attestation URL must be a plain HTTP(S) origin")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	local := host == "localhost" || host == "host.docker.internal" || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && (u.Scheme != "http" || !local) {
		return nil, errors.New("remote attestation requires HTTPS except for localhost or host.docker.internal")
	}
	if !validHelperToken(token) {
		return nil, errors.New("attestation helper token must be 32-512 non-whitespace characters")
	}
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("remote attestation requires a standard HTTP transport")
	}
	transport := defaultTransport.Clone()
	// Never send helper credentials through an ambient HTTP_PROXY/HTTPS_PROXY.
	transport.Proxy = nil
	return &remoteProvider{
		endpoint: strings.TrimRight(u.String(), "/"), token: token,
		client: &http.Client{Timeout: 8 * time.Second, Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

func validHelperToken(token string) bool {
	return len(token) >= 32 && len(token) <= 512 && !strings.ContainsAny(token, " \t\r\n")
}

func ReadHelperTokenFile(path string) (string, error) {
	// The operator configures this path; helper HTTP requests never supply it.
	file, err := os.Open(path) // #nosec G703 -- intentional operator-configured credential file
	if err != nil {
		return "", errors.New("cannot open attestation helper token file")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("attestation helper token file must be regular and owner-only (0600)")
	}
	data, err := io.ReadAll(io.LimitReader(file, 514))
	token := strings.TrimSpace(string(data))
	if err != nil || len(data) > 513 || !validHelperToken(token) {
		return "", errors.New("invalid attestation helper token file")
	}
	return token, nil
}

func (p *remoteProvider) Check(ctx context.Context) error {
	response, err := p.call(ctx, http.MethodGet, "/health")
	if err != nil {
		return err
	}
	if !response.Supported {
		return errors.New("remote attestation helper is not ready")
	}
	return nil
}

func (p *remoteProvider) Generate(ctx context.Context) (string, error) {
	response, err := p.call(ctx, http.MethodPost, "/v1/attestation")
	if err != nil {
		return "", err
	}
	if err := validateRemoteHeader(response.Attestation); err != nil {
		return "", err
	}
	return response.Attestation, nil
}

func (p *remoteProvider) call(ctx context.Context, method, path string) (remoteHelperResponse, error) {
	var result remoteHelperResponse
	req, err := http.NewRequestWithContext(ctx, method, p.endpoint+path, nil)
	if err != nil {
		return result, errors.New("cannot build remote attestation request")
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Accept", "application/json")
	response, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, errors.New("remote attestation helper request failed or timed out")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("remote attestation helper returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxHelperResponse+1))
	if err != nil || len(data) > maxHelperResponse {
		return result, errors.New("invalid remote attestation response size")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, errors.New("invalid remote attestation response")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return result, errors.New("invalid trailing remote attestation response")
	}
	return result, nil
}

func validateRemoteHeader(header string) error {
	var envelope struct {
		Version int    `json:"v"`
		Status  *int   `json:"s"`
		Token   string `json:"t"`
	}
	if len(header) < 20 || len(header) > 16*1024 || strings.ContainsAny(header, "\r\n") || json.Unmarshal([]byte(header), &envelope) != nil || envelope.Version != 1 || envelope.Status == nil || *envelope.Status != 0 || !strings.HasPrefix(envelope.Token, "v1.") || len(envelope.Token) <= 3 {
		return errors.New("remote helper returned malformed Live attestation")
	}
	return nil
}
