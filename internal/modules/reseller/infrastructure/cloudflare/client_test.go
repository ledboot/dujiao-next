package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/config"
	domain "github.com/dujiao-next/internal/modules/reseller/domain"
)

func TestProvisionOwnershipRecoveryAndCleanup(t *testing.T) {
	t.Setenv("SAAS_TEST_TOKEN", "secret")
	calls, creates, deletes := 0, 0, 0
	exists := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing authorization")
		}
		remote := map[string]any{"id": "remote-id", "hostname": "shop.customer.test", "status": "active", "ssl": map[string]any{"status": "active", "validation_records": []any{map[string]string{"txt_name": "_acme-challenge.shop.customer.test", "txt_value": "cf-value"}}}}
		var result any
		switch r.Method {
		case "GET":
			if strings.HasSuffix(r.URL.Path, "/remote-id") {
				result = remote
			} else if exists {
				result = []any{remote}
			} else {
				result = []any{}
			}
		case "POST":
			creates++
			exists = true
			result = remote
		case "DELETE":
			deletes++
			exists = false
			result = map[string]string{"id": "remote-id"}
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "result": result})
	}))
	defer srv.Close()
	c := New(config.CloudflareSaaSConfig{ZoneID: strings.Repeat("a", 32), CNAMETarget: "connect.platform.test", TokenEnv: "SAAS_TEST_TOKEN"})
	c.baseURL = srv.URL
	c.lookup = func(context.Context, string) (string, error) { return "connect.platform.test.", nil }
	owned := false
	c.lookupTXT = func(context.Context, string) ([]string, error) {
		if owned {
			return []string{"binding-token"}, nil
		}
		return nil, nil
	}
	state := domain.DomainHosting{Managed: true, OwnershipToken: "binding-token"}
	state, err := c.Reconcile(context.Background(), "shop.customer.test", state, false)
	if err != nil || calls != 0 || state.ErrorKey != "dns" {
		t.Fatalf("unowned domain must not reach provider: %+v %v calls=%d", state, err, calls)
	}
	owned = true
	state, err = c.Reconcile(context.Background(), "shop.customer.test", state, false)
	if err != nil || creates != 1 || !state.DNSReady || state.SSLStatus != "active" || len(state.Records) != 3 {
		t.Fatalf("provision: %+v %v", state, err)
	}
	// Lost local ID after remote success: recover without a duplicate create.
	state.HostnameID = ""
	state, err = c.Reconcile(context.Background(), "shop.customer.test", state, false)
	if err != nil || creates != 1 || state.HostnameID != "remote-id" {
		t.Fatalf("recovery: %+v %v", state, err)
	}
	// Removing the ownership record revokes readiness, even if Cloudflare remains active.
	owned = false
	state, err = c.Reconcile(context.Background(), "shop.customer.test", state, false)
	if err != nil || state.DNSReady {
		t.Fatal("stale provider activation bypassed ownership")
	}
	state, err = c.Reconcile(context.Background(), "shop.customer.test", state, true)
	if err != nil || deletes != 1 || state.HostnameID != "" || state.HostnameStatus != "deleted" {
		t.Fatalf("cleanup: %+v %v", state, err)
	}
}

func TestProviderErrorsDoNotExposeSecrets(t *testing.T) {
	t.Setenv("SAAS_TEST_TOKEN", "secret-token")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte("secret-token private details"))
	}))
	defer srv.Close()
	c := New(config.CloudflareSaaSConfig{ZoneID: strings.Repeat("a", 32), TokenEnv: "SAAS_TEST_TOKEN"})
	c.baseURL = srv.URL
	_, err := c.request(context.Background(), "GET", "", nil, nil)
	if !errors.Is(err, errProvider) || strings.Contains(err.Error(), "secret-token") {
		t.Fatal(err)
	}
}

func TestUnknownRemoteCannotBeDeletedWithoutBindingProof(t *testing.T) {
	t.Setenv("SAAS_TEST_TOKEN", "secret")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unowned resource mutation: %s", r.Method)
		}
		w.Write([]byte(`{"success":true,"result":[{"id":"existing","hostname":"shop.customer.test","status":"active"}]}`))
	}))
	defer srv.Close()
	c := New(config.CloudflareSaaSConfig{ZoneID: strings.Repeat("a", 32), CNAMETarget: "connect.platform.test", TokenEnv: "SAAS_TEST_TOKEN"})
	c.baseURL = srv.URL
	c.lookupTXT = func(context.Context, string) ([]string, error) { return nil, nil }
	state, err := c.Reconcile(context.Background(), "shop.customer.test", domain.DomainHosting{OwnershipToken: "new-binding"}, true)
	if err == nil || state.HostnameID != "" {
		t.Fatalf("adopted unowned resource: %+v %v", state, err)
	}
}

func TestPrivateHostingFieldsAreNotSerialized(t *testing.T) {
	data, err := json.Marshal(domain.DomainHosting{ZoneID: "private-zone", HostnameID: "private-id", OwnershipToken: "private-token"})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"private-zone", "private-id", "private-token", "LeaseUntil", "NextCheckAt"} {
		if strings.Contains(string(data), value) {
			t.Fatalf("private field exposed: %s", data)
		}
	}
}
