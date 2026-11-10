package publicconfighttp

import (
	"context"
	"encoding/json"
	"fmt"
	reseller "github.com/dujiao-next/internal/modules/reseller/contract"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type googleAuthPublicStub map[string]interface{}

func (s googleAuthPublicStub) PublicConfig() map[string]interface{} {
	return map[string]interface{}(s)
}

func TestResolveGoogleAuthPublicConfig(t *testing.T) {
	tests := []struct {
		name     string
		source   GoogleAuthPublic
		fallback GoogleAuthFallback
		want     map[string]interface{}
	}{
		{
			name:     "fallback enabled",
			fallback: GoogleAuthFallback{Enabled: true, ClientID: " fallback-client "},
			want:     map[string]interface{}{"enabled": true, "client_id": "fallback-client"},
		},
		{
			name:     "service overrides fallback",
			source:   googleAuthPublicStub{"enabled": true, "client_id": " runtime-client ", "unexpected": "private"},
			fallback: GoogleAuthFallback{Enabled: false, ClientID: "fallback-client"},
			want:     map[string]interface{}{"enabled": true, "client_id": "runtime-client"},
		},
		{
			name:     "missing client id fails closed",
			source:   googleAuthPublicStub{"enabled": true, "client_id": ""},
			fallback: GoogleAuthFallback{Enabled: true, ClientID: "fallback-client"},
			want:     map[string]interface{}{"enabled": false, "client_id": ""},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveGoogleAuthPublicConfig(test.source, test.fallback); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("public config = %#v, want %#v", got, test.want)
			}
		})
	}
}

type warmConfigCache struct{ keys []string }

func (s *warmConfigCache) CacheKey(id *uint) string {
	if id == nil {
		return "main"
	}
	return fmt.Sprintf("reseller:%d", *id)
}
func (s *warmConfigCache) GetJSON(_ context.Context, key string, out *map[string]interface{}) (bool, error) {
	s.keys = append(s.keys, key)
	*out = map[string]interface{}{"brand": map[string]interface{}{"site_name": "MAIN_ONLY"}}
	return true, nil
}
func (*warmConfigCache) SetJSON(context.Context, string, interface{}, time.Duration) error {
	return nil
}

type currentOverlay struct{ name string }

func (s *currentOverlay) ApplyPublicConfigOverlay(_ context.Context, tenant reseller.TenantContext, base map[string]interface{}) (map[string]interface{}, error) {
	if tenant.IsReseller() {
		base["brand"] = map[string]interface{}{"site_name": s.name, "site_url": "https://" + tenant.Host}
	}
	return base, nil
}

func TestCachedConfigStillAppliesCurrentTenantAndHost(t *testing.T) {
	cache := &warmConfigCache{}
	overlay := &currentOverlay{}
	h := &Handler{cache: cache, overlay: overlay}
	for _, tc := range []struct {
		host, name string
		id         uint
	}{
		{"first.example", "First", 7}, {"alias.example", "Changed", 7}, {"second.example", "Second", 8},
	} {
		overlay.name = tc.name
		ctx := reseller.WithTenantContext(context.Background(), reseller.ResellerTenantContext(tc.host, tc.id, 1, tc.host))
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "https://"+tc.host+"/api/v1/public/config", nil).WithContext(ctx)
		h.GetConfig(c)
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || strings.Contains(w.Body.String(), "MAIN_ONLY") || !strings.Contains(w.Body.String(), tc.name) || !strings.Contains(w.Body.String(), "https://"+tc.host) {
			t.Fatalf("cache leaked/stale config: %s", w.Body.String())
		}
	}
	if strings.Join(cache.keys, ",") != "reseller:7,reseller:7,reseller:8" {
		t.Fatalf("unscoped cache keys: %v", cache.keys)
	}
}
