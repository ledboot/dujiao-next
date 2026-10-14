package epayadapter

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/modules/payment/infrastructure/gateway/epay"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"

	"github.com/shopspring/decimal"
)

func TestEpayAdapter_Type(t *testing.T) {
	a := NewEpayAdapter()
	want := constants.PaymentProviderEpay + ":"
	if got := a.Type(); got != want {
		t.Fatalf("Type() = %q, want %q", got, want)
	}
}

func TestEpayAdapter_ValidateConfig_UnsupportedChannel(t *testing.T) {
	a := NewEpayAdapter()
	err := a.ValidateConfig(jsonmap.JSON{}, "no-such-channel-type")
	if err == nil {
		t.Fatalf("expected error for unsupported channel")
	}
	if !errors.Is(err, paymentcontract.ErrGatewayUnsupportedChannel) {
		t.Fatalf("expected wrapped paymentcontract.ErrGatewayUnsupportedChannel, got %v", err)
	}
}

func TestEpayAdapter_CreatePayment_ConfigInvalidMapped(t *testing.T) {
	a := NewEpayAdapter()
	// 传一个 epay.IsSupportedChannelType 接受的 channelType(让校验过),
	// 但 config 空导致 ParseConfig/ValidateConfig 失败
	_, err := a.CreatePayment(context.Background(), jsonmap.JSON{}, paymentcontract.GatewayCreateInput{
		OrderNo:     "ORDER_1",
		Currency:    "CNY",
		ChannelType: "alipay", // epay 支持 alipay/wxpay/qqpay
	})
	if err == nil {
		t.Fatalf("expected error from empty config")
	}
	if !errors.Is(err, paymentcontract.ErrGatewayConfigInvalid) {
		t.Fatalf("expected wrapped paymentcontract.ErrGatewayConfigInvalid, got %v", err)
	}
}

// TestEpayAdapter_CreatePayment_ExchangeRate_AuditFields 守护 P1.2c audit
// 字段写入回归。模式见 stripe_adapter_test.go 同名测试。
func TestEpayAdapter_CreatePayment_ExchangeRate_AuditFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// epay native 期望 JSON-encoded-string 响应,这里直接返回原始 JSON 即可
		// (epay_create_test.go 用 double-encoded 测了 string→json 的退化路径,
		// 这里仅需走标准 path)
		_, _ = w.Write([]byte(`{"code":1,"msg":"success","trade_no":"EPAY-AUDIT-001","payurl":"https://pay.example.com/audit"}`))
	}))
	defer server.Close()

	a := NewEpayAdapter()
	raw := jsonmap.JSON{
		"gateway_url":  server.URL,
		"epay_version": "v1",
		"merchant_id":  "M-AUDIT",
		"merchant_key": "K-AUDIT-001",
		"notify_url":   "https://api.example.com/api/v1/payments/callback/epay",
		"return_url":   "https://shop.example.com/pay",
		"sign_type":    "MD5",
		// 跨币种:10 USD → 72 CNY (rate 7.2)
		"target_currency": "CNY",
		"exchange_rate":   "7.2",
	}

	input := paymentcontract.GatewayCreateInput{
		OrderNo:     "ORDER-EPAY-USD-10",
		Subject:     "audit field test",
		Currency:    "USD",
		Amount:      money.FromDecimal(decimal.NewFromInt(10)),
		ChannelType: "alipay", // epay 支持 alipay/wxpay/qqpay
		ClientIP:    "127.0.0.1",
		Extra:       jsonmap.JSON{"interaction_mode": constants.PaymentInteractionQR},
	}

	result, err := a.CreatePayment(context.Background(), raw, input)
	if err != nil {
		t.Fatalf("CreatePayment() failed: %v", err)
	}

	if result.CurrencySent != "CNY" {
		t.Fatalf("CurrencySent = %q, want CNY (converted target)", result.CurrencySent)
	}
	if result.AmountSent != "72" {
		t.Fatalf("AmountSent = %q, want 72 (10 USD * 7.2)", result.AmountSent)
	}

	if got := result.Payload["exchange_rate"]; got != "7.2" {
		t.Fatalf("Payload[exchange_rate] = %v, want 7.2", got)
	}
	if got := result.Payload["original_amount"]; got != "10" {
		t.Fatalf("Payload[original_amount] = %v, want 10", got)
	}
	if got := result.Payload["original_currency"]; got != "USD" {
		t.Fatalf("Payload[original_currency] = %v, want USD", got)
	}
}

func TestEpayAdapter_CreatePayment_V2RedirectUsesFormURL(t *testing.T) {
	privateKey, publicKey := generateAdapterRSAKeyPair(t)
	a := NewEpayAdapter()
	notifyURL := "https://api.example.com/api/v1/payments/callback?channel_id=3"
	raw := jsonmap.JSON{
		"gateway_url":         "https://gateway.example.com",
		"epay_version":        "v2",
		"merchant_id":         "1002",
		"private_key":         privateKey,
		"platform_public_key": publicKey,
		"notify_url":          notifyURL,
		"return_url":          "https://shop.example.com/pay?x=1&y=2",
	}
	result, err := a.CreatePayment(context.Background(), raw, paymentcontract.GatewayCreateInput{
		PaymentID:   42,
		OrderNo:     "DJP20002",
		Subject:     "测试订单2",
		Currency:    "CNY",
		Amount:      money.FromDecimal(decimal.RequireFromString("66.00")),
		ChannelType: constants.PaymentChannelTypeWxpay,
		Extra:       jsonmap.JSON{"interaction_mode": constants.PaymentInteractionRedirect},
	})
	if err != nil {
		t.Fatalf("CreatePayment() failed: %v", err)
	}

	parsed, err := url.Parse(result.RedirectURL)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	if got := parsed.Scheme + "://" + parsed.Host + parsed.Path; got != "https://shop.example.com/api/v1/payments/epay-redirect" {
		t.Fatalf("redirect url = %s", result.RedirectURL)
	}
	if got := parsed.Query().Get("payment_id"); got != "42" {
		t.Fatalf("payment_id = %s", got)
	}
	token := parsed.Query().Get("token")
	if token == "" {
		t.Fatalf("redirect url missing token: %s", result.RedirectURL)
	}
	if got, _ := result.Payload["token"].(string); got != token {
		t.Fatalf("payload token = %q, want %q", got, token)
	}
	params, _ := result.Payload["params"].(map[string]interface{})
	sign, _ := params["sign"].(string)
	if sign == "" {
		t.Fatalf("payload sign is empty")
	}
	if strings.Contains(result.RedirectURL, sign) || strings.Contains(result.RedirectURL, "sign=") {
		t.Fatalf("redirect url contains RSA sign: %s", result.RedirectURL)
	}
}

func TestEpayAdapter_CreatePayment_V2RedirectFailsClosed(t *testing.T) {
	privateKey, publicKey := generateAdapterRSAKeyPair(t)
	a := NewEpayAdapter()
	base := jsonmap.JSON{
		"gateway_url":         "https://gateway.example.com",
		"epay_version":        "v2",
		"merchant_id":         "1002",
		"private_key":         privateKey,
		"platform_public_key": publicKey,
		"notify_url":          "https://api.example.com/api/v1/payments/callback",
		"return_url":          "https://shop.example.com/pay",
	}
	input := paymentcontract.GatewayCreateInput{
		PaymentID:   42,
		OrderNo:     "DJP20002",
		Subject:     "测试订单2",
		Currency:    "CNY",
		Amount:      money.FromDecimal(decimal.RequireFromString("66.00")),
		ChannelType: constants.PaymentChannelTypeAlipay,
		Extra:       jsonmap.JSON{"interaction_mode": constants.PaymentInteractionRedirect},
	}

	input.PaymentID = 0
	if _, err := a.CreatePayment(context.Background(), base, input); !errors.Is(err, paymentcontract.ErrGatewayConfigInvalid) {
		t.Fatalf("zero payment id err = %v, want config invalid", err)
	}

	input.PaymentID = 42
	ftpReturn := jsonmap.JSON{}
	for key, value := range base {
		ftpReturn[key] = value
	}
	ftpReturn["return_url"] = "ftp://files.example.com/return"
	if _, err := a.CreatePayment(context.Background(), ftpReturn, input); !errors.Is(err, paymentcontract.ErrGatewayConfigInvalid) {
		t.Fatalf("non-http return origin err = %v, want config invalid", err)
	}
}

func TestEpayRedirectFormURLPrefersReturnURL(t *testing.T) {
	formURL, _, err := epayRedirectFormURL(7, "https://shop.example.com/pay", "https://api.example.com/callback")
	if err != nil {
		t.Fatalf("return_url origin: %v", err)
	}
	if !strings.HasPrefix(formURL, "https://shop.example.com/api/v1/payments/epay-redirect?") {
		t.Fatalf("formURL = %s, want return_url host", formURL)
	}

	formURL, _, err = epayRedirectFormURL(7, "", "https://api.example.com/callback")
	if err != nil {
		t.Fatalf("notify_url fallback: %v", err)
	}
	if !strings.HasPrefix(formURL, "https://api.example.com/api/v1/payments/epay-redirect?") {
		t.Fatalf("formURL = %s, want notify_url host", formURL)
	}
}

func generateAdapterRSAKeyPair(t *testing.T) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key failed: %v", err)
	}
	privateDER := x509.MarshalPKCS1PrivateKey(key)
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateDER})
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key failed: %v", err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	return string(privatePEM), string(publicPEM)
}

func TestEpayAdapter_VerifyCallbackRejectsMerchantMismatch(t *testing.T) {
	a := NewEpayAdapter()
	raw := jsonmap.JSON{
		"gateway_url":  "https://epay.example.com",
		"epay_version": "v1",
		"merchant_id":  "1001",
		"merchant_key": "key-001",
		"notify_url":   "https://api.example.com/api/v1/payments/callback/epay",
		"return_url":   "https://shop.example.com/pay",
		"sign_type":    "MD5",
	}
	form := signEpayV1AdapterCallbackForm(map[string]string{
		"pid":          "1002",
		"out_trade_no": "ORDER-1001",
		"trade_no":     "EPAY-2001",
		"money":        "10.00",
		"trade_status": constants.EpayTradeStatusSuccess,
	}, "key-001")

	_, err := a.(paymentcontract.GatewayCallbackVerifier).VerifyCallback(raw, form, nil)
	if err == nil {
		t.Fatalf("expected callback ownership error")
	}
	if !errors.Is(err, paymentcontract.ErrGatewaySignatureInvalid) {
		t.Fatalf("expected wrapped paymentcontract.ErrGatewaySignatureInvalid, got %v", err)
	}
}

func TestEpayAdapter_MapEpayError(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want error
	}{
		{"config", epay.ErrConfigInvalid, paymentcontract.ErrGatewayConfigInvalid},
		{"channel_type→unsupported", epay.ErrChannelTypeNotOK, paymentcontract.ErrGatewayUnsupportedChannel},
		{"sign_generate→config", epay.ErrSignatureGenerate, paymentcontract.ErrGatewayConfigInvalid},
		{"sign_invalid→signature", epay.ErrSignatureInvalid, paymentcontract.ErrGatewaySignatureInvalid},
		{"request", epay.ErrRequestFailed, paymentcontract.ErrGatewayRequestFailed},
		{"response", epay.ErrResponseInvalid, paymentcontract.ErrGatewayResponseInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapEpayError(tc.in)
			if !errors.Is(got, tc.want) {
				t.Fatalf("mapEpayError(%v) errors.Is %v = false, want true", tc.in, tc.want)
			}
		})
	}
}

func signEpayV1AdapterCallbackForm(params map[string]string, merchantKey string) map[string][]string {
	form := make(map[string][]string, len(params)+2)
	for key, value := range params {
		form[key] = []string{value}
	}
	form["sign_type"] = []string{"MD5"}
	form["sign"] = []string{md5Hex(buildEpayAdapterSignContent(params) + merchantKey)}
	return form
}

func buildEpayAdapterSignContent(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if value == "" || key == "sign" || key == "sign_type" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%s", key, params[key]))
	}
	return strings.Join(pairs, "&")
}

func md5Hex(content string) string {
	sum := md5.Sum([]byte(content))
	return strings.ToLower(hex.EncodeToString(sum[:]))
}

func TestMolaRedirect(t *testing.T) {
	a := NewMolaAdapter()
	raw := jsonmap.JSON{"merchant_id": "1001", "merchant_key": "mola-test-key", "notify_url": "https://shop.example/api/v1/payments/callback", "return_url": "https://shop.example/pay"}
	for channel, want := range map[string]string{"wechat": "wxpay", "alipay": "alipay", "qqpay": "qqpay", "tenpay": "tenpay"} {
		t.Run(channel, func(t *testing.T) {
			result, err := a.CreatePayment(context.Background(), raw, paymentcontract.GatewayCreateInput{OrderNo: "DJP001", Subject: "测试 & 商品", Amount: money.FromDecimal(decimal.NewFromInt(10)), Currency: "CNY", ChannelType: channel, Extra: jsonmap.JSON{"interaction_mode": "redirect"}, ReturnURLQuery: map[string]string{"mola_return": "1", "order_no": "ORDER001"}})
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(result.RedirectURL)
			if err != nil {
				t.Fatal(err)
			}
			if u.Scheme != "https" || u.Host != "pay.mygptlife.com" || u.Path != "/submit.php" {
				t.Fatalf("unexpected URL: %s", u)
			}
			q := u.Query()
			if q.Get("type") != want || q.Get("pid") != "1001" || q.Get("out_trade_no") != "DJP001" || q.Get("money") != "10" || q.Get("name") != "测试 & 商品" || q.Get("sign_type") != "MD5" {
				t.Fatalf("unexpected params: %v", q)
			}
			params := map[string]string{}
			for k := range q {
				params[k] = q.Get(k)
			}
			if q.Get("sign") != md5Hex(buildEpayAdapterSignContent(params)+"mola-test-key") {
				t.Fatal("invalid outgoing signature")
			}
			back, _ := url.Parse(q.Get("return_url"))
			if back.Query().Get("mola_return") != "1" || back.Query().Get("order_no") != "ORDER001" {
				t.Fatal("missing return tracking")
			}
			if raw["epay_version"] != nil || raw["gateway_url"] != nil {
				t.Fatal("mutated channel config")
			}
		})
	}
	for _, tc := range []struct {
		name     string
		extra    jsonmap.JSON
		currency string
		config   jsonmap.JSON
	}{
		{"qr", jsonmap.JSON{"interaction_mode": "qr"}, "CNY", nil},
		{"currency", jsonmap.JSON{"interaction_mode": "redirect"}, "USD", nil},
		{"v2", jsonmap.JSON{"interaction_mode": "redirect"}, "CNY", jsonmap.JSON{"epay_version": "v2"}},
		{"rsa", jsonmap.JSON{"interaction_mode": "redirect"}, "CNY", jsonmap.JSON{"sign_type": "RSA"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := jsonmap.JSON{}
			for k, v := range raw {
				cfg[k] = v
			}
			for k, v := range tc.config {
				cfg[k] = v
			}
			_, err := a.CreatePayment(context.Background(), cfg, paymentcontract.GatewayCreateInput{OrderNo: "DJP001", Amount: money.FromDecimal(decimal.NewFromInt(10)), Currency: tc.currency, ChannelType: "alipay", Extra: tc.extra})
			if !errors.Is(err, paymentcontract.ErrGatewayConfigInvalid) {
				t.Fatalf("expected config error, got %v", err)
			}
		})
	}
}

func TestMolaCurrencyConversionAndNotificationStatus(t *testing.T) {
	a := NewMolaAdapter()
	raw := jsonmap.JSON{"merchant_id": "1001", "merchant_key": "mola-key", "notify_url": "https://shop.example/api/v1/payments/callback", "return_url": "https://shop.example/pay", "target_currency": "CNY", "exchange_rate": "7"}
	result, err := a.CreatePayment(context.Background(), raw, paymentcontract.GatewayCreateInput{OrderNo: "DJP001", Amount: money.FromDecimal(decimal.NewFromInt(10)), Currency: "USD", ChannelType: "alipay", Extra: jsonmap.JSON{"interaction_mode": "redirect"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.AmountSent != "70" || result.CurrencySent != "CNY" || result.Payload["original_currency"] != "USD" {
		t.Fatalf("conversion: %+v", result)
	}
	for _, status := range []string{"TRADE_SUCCESS", "TRADE_FINISHED", "WAIT_BUYER_PAY"} {
		form := signEpayV1AdapterCallbackForm(map[string]string{"pid": "1001", "out_trade_no": "DJP001", "trade_no": "MOLA001", "trade_status": status, "money": "70.00"}, "mola-key")
		callback, err := a.(paymentcontract.GatewayCallbackVerifier).VerifyCallback(raw, form, nil)
		if err != nil {
			t.Fatal(err)
		}
		if (callback.Status == constants.PaymentStatusSuccess) != (status == "TRADE_SUCCESS") {
			t.Fatalf("unexpected status for %s: %s", status, callback.Status)
		}
	}
}
