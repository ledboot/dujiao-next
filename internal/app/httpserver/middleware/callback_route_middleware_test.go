package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dujiao-next/internal/constants"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	paymenttransport "github.com/dujiao-next/internal/modules/payment/transport/http"
	settingsintegration "github.com/dujiao-next/internal/modules/settings/schema/integration"
	"github.com/gin-gonic/gin"
)

type callbackSettingsStub struct {
	routes settingsintegration.CallbackRoutesSetting
}

func (s callbackSettingsStub) GetCallbackRoutesCached() *settingsintegration.CallbackRoutesSetting {
	return &s.routes
}

type callbackWebhookStub struct{ calls int }

func (s *callbackWebhookStub) HandleStripeWebhook(paymenttransport.WebhookCallbackInput) (*paymentdomain.Payment, string, error) {
	s.calls++
	return nil, "", nil
}
func (s *callbackWebhookStub) HandlePaypalWebhook(input paymenttransport.WebhookCallbackInput) (*paymentdomain.Payment, string, error) {
	return s.HandleStripeWebhook(input)
}
func (s *callbackWebhookStub) HandleDujiaoPayWebhook(input paymenttransport.WebhookCallbackInput) (*paymentdomain.Payment, string, error) {
	return s.HandleStripeWebhook(input)
}

func TestCustomCallbackRoutesEnforceRateLimitBeforeDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	routes := settingsintegration.CallbackRoutesSetting{PaymentCallback: "/api/custom/payment", DujiaoPayWebhook: "/api/custom/dujiaopay", PaypalWebhook: "/api/custom/paypal", StripeWebhook: "/api/custom/stripe", UpstreamCallback: "/api/custom/upstream"}
	for _, path := range []string{routes.PaymentCallback, routes.DujiaoPayWebhook, routes.PaypalWebhook, routes.StripeWebhook, routes.UpstreamCallback} {
		t.Run(path, func(t *testing.T) {
			guard := RateLimitGuard(nil, RateLimitRule{WindowSeconds: 60, MaxRequests: 1}, KeyByIP)
			// 用同一来源耗尽限额。分发时必须在访问 handler 或 service 前拒绝。
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, path, nil)
			guard(c)
			if c.IsAborted() {
				t.Fatal("first request unexpectedly blocked")
			}
			r := gin.New()
			r.Use(CallbackRouteMiddleware(callbackSettingsStub{routes}, nil, paymenttransport.NewWebhookHandler(&callbackWebhookStub{}, nil), nil, guard))
			downstream := false
			r.NoRoute(func(c *gin.Context) { downstream = true; c.Status(http.StatusNotFound) })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
			if w.Code != http.StatusTooManyRequests || downstream {
				t.Fatalf("limit bypassed: status=%d downstream=%v", w.Code, downstream)
			}
		})
	}
}

func TestCustomCallbackRateLimitDoesNotRunOtherHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &callbackWebhookStub{}
	webhooks := paymenttransport.NewWebhookHandler(service, nil)
	guard := RateLimitGuard(nil, RateLimitRule{WindowSeconds: 60, MaxRequests: 1}, KeyByIP)
	r := gin.New()
	r.Use(CallbackRouteMiddleware(callbackSettingsStub{settingsintegration.CallbackRoutesSetting{StripeWebhook: "/api/custom/stripe"}}, nil, webhooks, nil, guard))
	downstream := 0
	r.NoRoute(func(c *gin.Context) { downstream++; c.Status(http.StatusNotFound) })
	r.GET("/api/ping", func(c *gin.Context) { c.Status(http.StatusOK) })
	for i, want := range []int{http.StatusOK, http.StatusTooManyRequests} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/custom/stripe", nil))
		if w.Code != want {
			t.Fatalf("request %d: status=%d body=%s", i, w.Code, w.Body.String())
		}
	}
	if service.calls != 1 || downstream != 0 {
		t.Fatalf("unexpected dispatch: service=%d downstream=%d", service.calls, downstream)
	}
	for _, tc := range []struct {
		path string
		want int
	}{{"/api/ping", http.StatusOK}, {constants.DefaultStripeWebhookPath, http.StatusNotFound}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.want {
			t.Fatalf("%s: status=%d", tc.path, w.Code)
		}
	}
}
