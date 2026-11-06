package captchahttp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/i18n"
	captchacontract "github.com/dujiao-next/internal/modules/captcha/contract"
	"github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/gin-gonic/gin"
)

const (
	storefrontCookie = "dj_storefront_clearance"
	storefrontTTL    = 24 * time.Hour
)

type storefrontVerifier interface {
	StorefrontSiteKey() (string, error)
	VerifyStorefront(token, clientIP, hostname string) (string, error)
}

// StorefrontGate issues and validates a host-bound browser clearance.
type StorefrontGate struct {
	verifier storefrontVerifier
	secret   string
}

func NewStorefrontGate(verifier storefrontVerifier, secret string) StorefrontGate {
	return StorefrontGate{verifier: verifier, secret: secret}
}

func (g StorefrontGate) RegisterRoutes(public gin.IRoutes, limit gin.HandlerFunc) {
	public.GET("/captcha/storefront", g.status)
	public.POST("/captcha/storefront", limit, g.verify)
}

func (g StorefrontGate) status(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	key, err := g.verifier.StorefrontSiteKey()
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.captcha_unavailable", err)
		return
	}
	if key == "" || g.hasClearance(c, key) {
		response.Success(c, gin.H{"required": false})
		return
	}
	response.Success(c, gin.H{"required": true, "site_key": key, "action": captchacontract.StorefrontAction})
}

func (g StorefrontGate) verify(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	var req CaptchaPayloadRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&req); err != nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.captcha_invalid", nil)
		return
	}
	host := storefrontHostname(c)
	key, err := g.verifier.VerifyStorefront(req.TurnstileToken, c.ClientIP(), host)
	switch {
	case errors.Is(err, captchacontract.ErrRequired):
		ginutil.RespondError(c, response.CodeBadRequest, "error.captcha_required", nil)
	case errors.Is(err, captchacontract.ErrInvalid):
		ginutil.RespondError(c, response.CodeBadRequest, "error.captcha_invalid", nil)
	case err != nil || g.secret == "":
		ginutil.RespondError(c, response.CodeInternal, "error.captcha_unavailable", err)
	default:
		expires := time.Now().Add(storefrontTTL)
		deadline := strconv.FormatInt(expires.Unix(), 10)
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     storefrontCookie,
			Value:    deadline + "." + g.signature(host, key, deadline),
			Path:     "/api/v1",
			Expires:  expires,
			MaxAge:   int(storefrontTTL.Seconds()),
			HttpOnly: true,
			Secure:   c.Request.TLS != nil || (host != "localhost" && host != "127.0.0.1" && host != "::1"),
			SameSite: http.SameSiteLaxMode,
		})
		response.Success(c, nil)
	}
}

// Require protects storefront APIs without intercepting protocol callbacks.
func (g StorefrontGate) Require(c *gin.Context) {
	key, err := g.verifier.StorefrontSiteKey()
	if err == nil && key == "" {
		c.Next()
		return
	}
	c.Header("Cache-Control", "private, no-store")
	if err != nil {
		response.ErrorWithHTTPStatus(c, http.StatusServiceUnavailable, response.CodeInternal,
			i18n.T(i18n.ResolveLocale(c), "error.captcha_unavailable"))
		return
	}
	if !g.hasClearance(c, key) {
		response.ErrorWithHTTPStatus(c, http.StatusPreconditionRequired, response.CodeForbidden,
			i18n.T(i18n.ResolveLocale(c), "error.captcha_required"))
		return
	}
	c.Next()
}

func (g StorefrontGate) hasClearance(c *gin.Context, key string) bool {
	cookie, err := c.Request.Cookie(storefrontCookie)
	if err != nil || g.secret == "" {
		return false
	}
	deadline, signature, _ := strings.Cut(cookie.Value, ".")
	expires, err := strconv.ParseInt(deadline, 10, 64)
	return err == nil && time.Now().Unix() < expires &&
		hmac.Equal([]byte(signature), []byte(g.signature(storefrontHostname(c), key, deadline)))
}

func (g StorefrontGate) signature(host, key, deadline string) string {
	mac := hmac.New(sha256.New, []byte(g.secret))
	_, _ = io.WriteString(mac, storefrontCookie+"\x00"+host+"\x00"+key+"\x00"+deadline)
	return hex.EncodeToString(mac.Sum(nil))
}

func storefrontHostname(c *gin.Context) string {
	host := url.URL{Host: c.Request.Host}
	return strings.ToLower(strings.TrimSuffix(host.Hostname(), "."))
}
