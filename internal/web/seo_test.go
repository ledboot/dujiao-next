package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestPublicHTMLSanitization(t *testing.T) {
	raw := `<h2>Guide</h2><p onclick="steal()">Text <a href="https://example.com/help">source</a></p><script>steal()</script><iframe src="https://bad.example"></iframe><svg onload="steal()"></svg><img src="javascript:steal()" onerror="steal()"><a href="jav&#x61;script:steal()">bad</a><a href="//evil.example">bad</a><form><input value="secret"></form>`
	clean := sanitizePublicHTML(raw)
	for _, forbidden := range []string{"steal", "onclick", "onerror", "<script", "<iframe", "<svg", "<form", "secret", "//evil"} {
		if strings.Contains(clean, forbidden) {
			t.Errorf("unsafe content retained %q: %s", forbidden, clean)
		}
	}
	if !strings.Contains(clean, `<h2>Guide</h2>`) || !strings.Contains(clean, `href="https://example.com/help"`) {
		t.Fatalf("readable content lost: %s", clean)
	}
}

func TestRenderedPublicPageAndScriptEscaping(t *testing.T) {
	index := []byte(`<!doctype html><html lang="en"><head><title></title></head><body><div id="app"></div></body></html>`)
	page := PublicPage{Path: "/", Locale: "zh-CN", SiteName: "Brand", Title: `Brand </title><script>bad()</script>`, Heading: "Brand", Description: `</script><script>bad()</script>`, Canonical: "https://shop.example/", Type: "WebSite", Body: "<h2>Public information</h2>"}
	result, err := renderPublicPage(index, page)
	if err != nil {
		t.Fatal(err)
	}
	body := string(result)
	for _, want := range []string{`<html lang="zh-CN"`, `rel="canonical" href="https://shop.example/"`, `id="storefront-public-content"`, `Public information`, `"@type":"Organization"`, `\u003c/script\u003e`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, `<script>bad()`) {
		t.Fatalf("script escaped JSON or title: %s", body)
	}
	if strings.Count(body, `rel="canonical"`) != 1 {
		t.Fatal("canonical must be unique")
	}
	if strings.Index(body, `id="storefront-public-content"`) > strings.Index(body, `id="app"`) {
		t.Fatal("public content must precede the verification interface")
	}
}

func TestRenderedAboutPageLinksBrandEntity(t *testing.T) {
	index := []byte(`<!doctype html><html lang="zh-CN"><head><title></title></head><body><div id="app"></div></body></html>`)
	page := PublicPage{Path: "/about", Locale: "zh-CN", SiteName: "云柚AI", Title: "关于云柚AI", Heading: "关于云柚AI", Description: "官方站点说明", Canonical: "https://shop.example/about", Type: "WebPage"}
	result, err := renderPublicPage(index, page)
	if err != nil {
		t.Fatal(err)
	}
	body := string(result)
	for _, want := range []string{
		`"@type":"Organization"`,
		`"alternateName":"云柚 AI"`,
		`"@type":"AboutPage"`,
		`"mainEntity":{"@id":"https://shop.example/#organization"}`,
		`"contactPoint"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s: %s", want, body)
		}
	}
}

func TestPublicPageHTTPStatusAndStaticAssets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fsys := fstest.MapFS{"index.html": {Data: []byte(`<html lang="en"><head><title></title></head><body><div id="app"></div></body></html>`)}, "assets/main.js": {Data: []byte("asset")}}
	r := gin.New()
	calls := 0
	err := RegisterUser(r, fsys, func(req *http.Request) (*PublicPage, error) {
		calls++
		if req.URL.Path == "/unavailable" {
			return nil, errors.New("database unavailable")
		}
		return &PublicPage{Path: req.URL.Path, Locale: "zh-CN", Title: "Missing", Status: 404}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path   string
		status int
	}{{"/blog/draft", 404}, {"/unavailable", 503}, {"/api/unknown", 404}, {"/assets/main.js", 200}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status {
			t.Errorf("%s: got %d", tc.path, w.Code)
		}
		if tc.path == "/blog/draft" && (!strings.Contains(w.Body.String(), "noindex, follow") || strings.Contains(w.Body.String(), `rel="canonical"`)) {
			t.Fatal("404 must not advertise an indexable canonical")
		}
	}
	if calls != 2 {
		t.Fatalf("static assets and reserved APIs must not invoke SEO loader: %d", calls)
	}
}
