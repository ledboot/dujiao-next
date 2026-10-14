package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/dujiao-next/internal/config"
	domain "github.com/dujiao-next/internal/modules/reseller/domain"
)

var errProvider = errors.New("cloudflare domain request failed")
var identifier = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)

type Client struct {
	lookupTXT func(context.Context, string) ([]string, error)
	cfg       config.CloudflareSaaSConfig
	http      *http.Client
	baseURL   string
	lookup    func(context.Context, string) (string, error)
}

func New(cfg config.CloudflareSaaSConfig) *Client {
	return &Client{lookupTXT: net.DefaultResolver.LookupTXT, cfg: cfg, http: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, baseURL: "https://api.cloudflare.com/client/v4", lookup: net.DefaultResolver.LookupCNAME}
}

type hostname struct {
	ID        string `json:"id"`
	Hostname  string `json:"hostname"`
	Status    string `json:"status"`
	Ownership struct {
		Type  string `json:"type"`
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"ownership_verification"`
	SSL struct {
		Status  string `json:"status"`
		Records []struct {
			TXTName  string `json:"txt_name"`
			TXTValue string `json:"txt_value"`
		} `json:"validation_records"`
	} `json:"ssl"`
}

// request deliberately never returns response bodies or credentials in errors.
func (c *Client) request(ctx context.Context, method, path string, body any, out any) (int, error) {
	env := c.cfg.TokenEnv
	if env == "" {
		env = "DUJIAO_CLOUDFLARE_API_TOKEN"
	}
	token := strings.TrimSpace(os.Getenv(env))
	if token == "" || !identifier.MatchString(c.cfg.ZoneID) {
		return 0, errProvider
	}
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/zones/"+c.cfg.ZoneID+"/custom_hostnames"+path, bytes.NewReader(data))
	if err != nil {
		return 0, errProvider
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return 0, errProvider
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return res.StatusCode, errProvider
	}
	var envelope struct {
		Success bool            `json:"success"`
		Result  json.RawMessage `json:"result"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&envelope) != nil || !envelope.Success {
		return res.StatusCode, errProvider
	}
	if out != nil && json.Unmarshal(envelope.Result, out) != nil {
		return res.StatusCode, errProvider
	}
	return res.StatusCode, nil
}

func (c *Client) Reconcile(ctx context.Context, host string, state domain.DomainHosting, remove bool) (domain.DomainHosting, error) {
	state.ErrorKey = ""
	if state.ZoneID != "" && state.ZoneID != c.cfg.ZoneID {
		state.ErrorKey = "configuration"
		return state, errProvider
	}
	if state.CNAMETarget == "" {
		state.CNAMETarget = strings.TrimSuffix(strings.ToLower(c.cfg.CNAMETarget), ".")
	}
	if state.CNAMETarget == "" || strings.ContainsAny(state.CNAMETarget, "/: *") {
		state.ErrorKey = "configuration"
		return state, errProvider
	}
	state.ZoneID = c.cfg.ZoneID
	if len(state.Records) == 0 {
		state.Records = []domain.DNSRecord{{Type: "CNAME", Name: host, Value: state.CNAMETarget}, {Type: "TXT", Name: "_dujiao-verification." + host, Value: state.OwnershipToken}}
	}
	// Before creating/adopting a provider resource, prove this particular local binding.
	// This prevents taking over an already-active hostname in the same Cloudflare zone.
	owned := false
	tokens, txtErr := c.lookupTXT(ctx, "_dujiao-verification."+host)
	for _, token := range tokens {
		if state.OwnershipToken != "" && token == state.OwnershipToken {
			owned = true
		}
	}
	owned = owned && txtErr == nil
	if !remove && !owned && state.HostnameID == "" {
		state.DNSReady = false
		state.ErrorKey = "dns"
		return state, nil
	}
	var remote hostname
	// Exact lookup recovers a create that succeeded remotely but timed out locally.
	if state.HostnameID == "" {
		var rows []hostname
		if _, err := c.request(ctx, http.MethodGet, "?hostname="+url.QueryEscape(host), nil, &rows); err != nil {
			return state, err
		}
		for _, row := range rows {
			if strings.EqualFold(row.Hostname, host) {
				remote = row
				if remove && !owned {
					state.ErrorKey = "dns"
					return state, errProvider
				}
				state.HostnameID = row.ID
				break
			}
		}
	}
	if remove {
		if state.HostnameID != "" {
			code, err := c.request(ctx, http.MethodDelete, "/"+url.PathEscape(state.HostnameID), nil, nil)
			if err != nil && code != http.StatusNotFound {
				return state, err
			}
		}
		state.HostnameID = ""
		state.HostnameStatus = "deleted"
		state.SSLStatus = ""
		state.Records = nil
		state.DNSReady = false
		return state, nil
	}
	if state.HostnameID == "" {
		body := map[string]any{"hostname": host, "ssl": map[string]any{"method": "txt", "type": "dv", "settings": map[string]string{"min_tls_version": "1.2"}}}
		if _, err := c.request(ctx, http.MethodPost, "", body, &remote); err != nil {
			return state, err
		}
		state.HostnameID = remote.ID
	} else {
		code, err := c.request(ctx, http.MethodGet, "/"+url.PathEscape(state.HostnameID), nil, &remote)
		if err != nil {
			if code == http.StatusNotFound {
				state.HostnameID = ""
			}
			return state, err
		}
	}
	if remote.ID == "" || !strings.EqualFold(remote.Hostname, host) {
		return state, errProvider
	}
	if remote.SSL.Status == "validation_timed_out" {
		body := map[string]any{"ssl": map[string]string{"method": "txt", "type": "dv"}}
		if _, err := c.request(ctx, http.MethodPatch, "/"+url.PathEscape(state.HostnameID), body, &remote); err != nil {
			return state, err
		}
	}
	state.HostnameStatus = remote.Status
	state.SSLStatus = remote.SSL.Status
	state.Records = []domain.DNSRecord{{Type: "CNAME", Name: host, Value: state.CNAMETarget}, {Type: "TXT", Name: "_dujiao-verification." + host, Value: state.OwnershipToken}}
	if remote.Ownership.Type == "txt" && remote.Ownership.Name != "" {
		state.Records = append(state.Records, domain.DNSRecord{Type: "TXT", Name: remote.Ownership.Name, Value: remote.Ownership.Value})
	}
	for _, r := range remote.SSL.Records {
		if r.TXTName != "" {
			state.Records = append(state.Records, domain.DNSRecord{Type: "TXT", Name: r.TXTName, Value: r.TXTValue})
		}
	}
	canonical, err := c.lookup(ctx, host)
	state.DNSReady = err == nil && strings.EqualFold(strings.TrimSuffix(canonical, "."), state.CNAMETarget)
	state.DNSReady = state.DNSReady && owned
	if !state.DNSReady {
		state.ErrorKey = "dns"
	}
	if remote.Status == "blocked" || remote.Status == "deleted" || remote.SSL.Status == "validation_timed_out" || remote.SSL.Status == "expired" {
		state.ErrorKey = "validation"
	}
	return state, nil
}
