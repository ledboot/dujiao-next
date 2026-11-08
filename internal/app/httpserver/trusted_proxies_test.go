package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestConfigureTrustedProxiesRejectsSpoofedForwardedIPFromUntrustedPeer(t *testing.T) {
	engine := gin.New()
	if err := configureTrustedProxies(engine, nil); err != nil {
		t.Fatal(err)
	}
	engine.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	if resp.Body.String() != "203.0.113.9" {
		t.Fatalf("untrusted peer spoofed client IP: %q", resp.Body.String())
	}
}

func TestConfigureTrustedProxiesAcceptsForwardedIPFromConfiguredProxy(t *testing.T) {
	engine := gin.New()
	if err := configureTrustedProxies(engine, []string{"127.0.0.1/32"}); err != nil {
		t.Fatal(err)
	}
	engine.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	if resp.Body.String() != "1.2.3.4" {
		t.Fatalf("trusted proxy client IP=%q, want 1.2.3.4", resp.Body.String())
	}
}

func TestConfigureTrustedProxiesRejectsInvalidCIDR(t *testing.T) {
	if err := configureTrustedProxies(gin.New(), []string{"not-a-cidr"}); err == nil {
		t.Fatal("expected invalid trusted proxy configuration to fail")
	}
}

func TestConfigureTrustedProxiesRejectsTrustAllNetworks(t *testing.T) {
	for _, network := range []string{"0.0.0.0/0", "::/0"} {
		t.Run(network, func(t *testing.T) {
			if err := configureTrustedProxies(gin.New(), []string{network}); err == nil {
				t.Fatalf("expected %s to be rejected", network)
			}
		})
	}
}

func TestCloudflareClientIPHeaderRequiresTrustedPeer(t *testing.T) {
	for _, tc := range []struct{ peer, forwarded, want string }{
		{"172.18.0.6:1234", "198.51.100.23", "198.51.100.23"},
		{"172.18.0.9:1234", "2001:db8::23", "2001:db8::23"},
		{"203.0.113.9:1234", "198.51.100.23", "203.0.113.9"},
		{"172.18.0.8:1234", "198.51.100.23", "172.18.0.8"},
	} {
		engine := gin.New()
		if err := configureTrustedProxies(engine, []string{"172.18.0.6/32", "172.18.0.9/32"}); err != nil {
			t.Fatal(err)
		}
		engine.RemoteIPHeaders = []string{"CF-Connecting-IP", "X-Forwarded-For", "X-Real-IP"}
		engine.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
		req := httptest.NewRequest(http.MethodGet, "/ip", nil)
		req.RemoteAddr = tc.peer
		req.Header.Set("CF-Connecting-IP", tc.forwarded)
		req.Header.Set("X-Forwarded-For", "192.0.2.99")
		resp := httptest.NewRecorder()
		engine.ServeHTTP(resp, req)
		if resp.Body.String() != tc.want {
			t.Fatalf("peer=%s client=%s want=%s", tc.peer, resp.Body.String(), tc.want)
		}
	}
}
