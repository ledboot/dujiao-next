package captchahttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	captchacontract "github.com/dujiao-next/internal/modules/captcha/contract"
	"github.com/dujiao-next/internal/platform/http/response"

	"github.com/gin-gonic/gin"
)

type fakeGenerator struct {
	challenge *captchacontract.ImageChallenge
	err       error
}

func (f fakeGenerator) GenerateImageChallenge() (*captchacontract.ImageChallenge, error) {
	return f.challenge, f.err
}

func TestGetImageCaptchaSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewPublicHandler(fakeGenerator{challenge: &captchacontract.ImageChallenge{
		CaptchaID:   "id-1",
		ImageBase64: "data:image/png;base64,abc",
	}})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/captcha/image", nil)

	handler.GetImageCaptcha(c)

	var got response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.StatusCode != response.CodeOK {
		t.Fatalf("status want OK got %d body=%s", got.StatusCode, w.Body.String())
	}
	data, ok := got.Data.(map[string]interface{})
	if !ok || data["captcha_id"] != "id-1" {
		t.Fatalf("unexpected data %#v", got.Data)
	}
}

func TestGetImageCaptchaUnavailableWhenConfigInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewPublicHandler(fakeGenerator{err: captchacontract.ErrConfigInvalid})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/captcha/image", nil)

	handler.GetImageCaptcha(c)

	var got response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.StatusCode != response.CodeBadRequest {
		t.Fatalf("status want 400 got %d", got.StatusCode)
	}
}

func TestGetImageCaptchaUnavailableWithoutGenerator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewPublicHandler(nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/captcha/image", nil)

	handler.GetImageCaptcha(c)

	var got response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.StatusCode != response.CodeInternal {
		t.Fatalf("status want internal got %d", got.StatusCode)
	}
}

func TestGetImageCaptchaMapsUnknownErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewPublicHandler(fakeGenerator{err: errors.New("boom")})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/captcha/image", nil)

	handler.GetImageCaptcha(c)

	var got response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.StatusCode != response.CodeInternal {
		t.Fatalf("status want internal got %d", got.StatusCode)
	}
}

func TestStorefrontGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifier := &storefrontVerifierStub{siteKey: "site-key"}
	gate := NewStorefrontGate(verifier, "signing-key")
	router := gin.New()
	public := router.Group("/api/v1/public")
	gate.RegisterRoutes(public, func(c *gin.Context) { c.Next() })
	router.GET("/api/v1/public/products", gate.Require, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	const challengePath = "/api/v1/public/captcha/storefront"
	const productPath = "/api/v1/public/products"

	request := func(method, host, path, token string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "https://"+host+path, strings.NewReader(token))
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	status := request(http.MethodGet, "shop.example", challengePath, "", nil)
	if !strings.Contains(status.Body.String(), `"required":true`) ||
		!strings.Contains(status.Body.String(), `"site_key":"site-key"`) ||
		strings.Contains(status.Body.String(), "secret-key") ||
		status.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("unexpected challenge response: %s", status.Body)
	}
	if w := request(http.MethodGet, "shop.example", productPath, "", nil); w.Code != http.StatusPreconditionRequired {
		t.Fatalf("unverified request returned %d", w.Code)
	}
	for i, token := range []string{"invalid", "valid", "valid"} {
		w := request(http.MethodPost, "shop.example", challengePath, `{"turnstile_token":"`+token+`"}`, nil)
		cookies := w.Result().Cookies()
		if i != 1 {
			if len(cookies) != 0 || !strings.Contains(w.Body.String(), `"status_code":400`) {
				t.Fatalf("rejected token %q got %s, %d cookies", token, w.Body, len(cookies))
			}
			continue
		}
		if len(cookies) != 1 {
			t.Fatalf("valid token got %s, %d cookies", w.Body, len(cookies))
		}
		cookie := cookies[0]
		if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != 86400 || cookie.Domain != "" {
			t.Fatalf("unexpected cookie attributes: %v", cookie)
		}
		if w := request(http.MethodGet, "shop.example", productPath, "", cookie); w.Code != http.StatusNoContent {
			t.Fatalf("verified request returned %d", w.Code)
		}
		if w := request(http.MethodGet, "shop.example", challengePath, "", cookie); !strings.Contains(w.Body.String(), `"required":false`) {
			t.Fatalf("return visit got %s", w.Body)
		}
		expired := strconv.FormatInt(time.Now().Add(-time.Minute).Unix(), 10)
		for _, test := range []struct{ host, value string }{
			{"another.example", cookie.Value},
			{"shop.example", cookie.Value + "0"},
			{"shop.example", expired + "." + gate.signature("shop.example", "site-key", expired)},
		} {
			tampered := &http.Cookie{Name: cookie.Name, Value: test.value}
			if w := request(http.MethodGet, test.host, productPath, "", tampered); w.Code != http.StatusPreconditionRequired {
				t.Fatalf("invalid clearance returned %d", w.Code)
			}
		}
	}

	verifier.siteKey = ""
	if w := request(http.MethodGet, "shop.example", productPath, "", nil); w.Code != http.StatusNoContent {
		t.Fatalf("disabled gate returned %d", w.Code)
	}
}

type storefrontVerifierStub struct {
	siteKey string
	used    bool
}

func (s *storefrontVerifierStub) StorefrontSiteKey() (string, error) {
	return s.siteKey, nil
}

func (s *storefrontVerifierStub) VerifyStorefront(token, _, hostname string) (string, error) {
	if token != "valid" || hostname != "shop.example" || s.used {
		return "", captchacontract.ErrInvalid
	}
	s.used = true
	return s.siteKey, nil
}
