package collector

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mrlhansen/idrac_exporter/internal/config"
	"github.com/mrlhansen/idrac_exporter/internal/log"
	"golang.org/x/sync/semaphore"
)

type RedfishSession struct {
	disabled bool
	id       string
	token    string
}

type Redfish struct {
	baseurl  string
	hostname string
	username string
	password string
	session  RedfishSession
	http     *http.Client
	sem      *semaphore.Weighted
	ctx      context.Context
}

const redfishRootPath = "/redfish/v1"

func NewRedfish(host string, auth *config.AuthConfig) *Redfish {
	cfg := config.Config
	baseurl := fmt.Sprintf("%s://%s", auth.Scheme, host)
	if auth.Port > 0 {
		baseurl = fmt.Sprintf("%s:%d", baseurl, auth.Port)
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	// Enable legacy RSA key exchange only for explicitly configured targets.
	// TLS 1.0, TLS 1.1, 3DES, RC4, and CBC cipher suites remain disabled.
	if auth.AllowLegacyRSAKex {
		tlsConfig.CipherSuites = []uint16{
			// Modern TLS 1.2 AEAD cipher suites
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,

			// Legacy RSA key exchange restricted to AES-GCM cipher suites
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		}
	}

	return &Redfish{
		baseurl:  baseurl,
		hostname: host,
		username: auth.Username,
		password: auth.Password,
		session: RedfishSession{
			disabled: true,
		},
		http: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				TLSClientConfig:       tlsConfig,
				MaxIdleConnsPerHost:   int(cfg.Concurrency),                     // Allow more concurrent requests per host
				IdleConnTimeout:       30 * time.Second,                         // Remove stale connections after 30s
				ResponseHeaderTimeout: time.Duration(cfg.Timeout) * time.Second, // Timeout waiting for response headers
				ExpectContinueTimeout: 1 * time.Second,                          // Timeout for 100-continue responses
			},
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
		sem: semaphore.NewWeighted(int64(cfg.Concurrency)),
		ctx: context.Background(),
	}
}

func (r *Redfish) DisableSession() {
	r.session.disabled = true
}

func (r *Redfish) CreateSession() bool {
	// Stateless HTTP Basic Auth: do not create Redfish sessions on BMC
	log.Debug("Stateless HTTP Basic Auth active for %s (no Redfish session)", r.hostname)
	return true
}

func (r *Redfish) DeleteSession() bool {
	// Stateless HTTP Basic Auth: no session to delete
	return true
}

func (r *Redfish) RefreshSession() bool {
	// Stateless HTTP Basic Auth: no session refresh needed
	return true
}

func (r *Redfish) Get(path string, res any) bool {
	if !strings.HasPrefix(path, redfishRootPath) {
		return false
	}

	r.sem.Acquire(r.ctx, 1)
	defer r.sem.Release(1)

	url := fmt.Sprintf("%s%s", r.baseurl, path)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("OData-Version", "4.0")
	req.SetBasicAuth(r.username, r.password)

	log.Debug("Querying %q", url)
	resp, err := r.http.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		log.Error("Failed to query %q: %v", url, err)
		return false
	}

	if resp.StatusCode != http.StatusOK {
		log.Error("Unexpected status code from %q: %s", url, resp.Status)
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("Error reading response from %q: %v", url, err)
		return false
	}

	if config.Debug {
		log.Debug("Response from %q: %s", url, body)
	}

	// Issue #192
	body = bytes.ReplaceAll(body, []byte("\r"), []byte(""))

	err = json.Unmarshal(body, res)
	if err != nil {
		log.Error("Error decoding response from %q: %v", url, err)
		return false
	}

	return true
}

func (r *Redfish) Exists(path string) bool {
	if !strings.HasPrefix(path, redfishRootPath) {
		return false
	}

	url := fmt.Sprintf("%s%s", r.baseurl, path)
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return false
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("OData-Version", "4.0")
	req.SetBasicAuth(r.username, r.password)

	resp, err := r.http.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	if err != nil {
		return false
	}

	if resp.StatusCode >= 400 && resp.StatusCode <= 499 {
		return false
	}

	return true
}
