package epayadapter

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dujiao-next/internal/constants"
	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
)

func pay502v2TestConfig(merchantPrivateKey, platformPublicKey string) jsonmap.JSON {
	return jsonmap.JSON{
		"merchant_id":         "2002",
		"private_key":         merchantPrivateKey,
		"platform_public_key": platformPublicKey,
		"notify_url":          "https://shop.example/api/v1/payments/callback",
		"return_url":          "https://shop.example/pay",
	}
}

func TestPay502V2Type(t *testing.T) {
	if got := NewPay502V2Adapter().Type(); got != constants.PaymentProvider502V2+":" {
		t.Fatalf("Type() = %q", got)
	}
}

func TestPay502V2CreatePayment(t *testing.T) {
	merchantPrivate, merchantPublic := generateAdapterRSAKeyPair(t)
	platformPrivate, platformPublic := generateAdapterRSAKeyPair(t)
	for _, tc := range []struct {
		mode       string
		method     string
		payType    string
		payInfo    string
		wantQR     bool
		channelTyp string
	}{
		{mode: "qr", method: "scan", payType: "qrcode", payInfo: "https://pay.example/502v2-qr", wantQR: true, channelTyp: "alipay"},
		{mode: "redirect", method: "jump", payType: "jump", payInfo: "https://pay.example/502v2-jump", channelTyp: "wxpay"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/xpay/epay/api/pay/create" {
					t.Errorf("unexpected endpoint %s %s", r.Method, r.URL.Path)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				params := map[string]string{}
				for key := range r.PostForm {
					params[key] = r.PostForm.Get(key)
				}
				if params["sign_type"] != "RSA" || params["method"] != tc.method || params["pid"] != "2002" || params["type"] != tc.channelTyp || params["money"] != "70" {
					t.Errorf("unexpected request: %v", params)
				}
				if !verifyTestRSASignature(buildEpayAdapterSignContent(params), params["sign"], merchantPublic) {
					t.Error("invalid request signature")
				}
				respSign := signTestRSA(buildEpayAdapterSignContent(map[string]string{
					"code": "0", "msg": "success", "trade_no": "502V2-001", "pay_type": tc.payType, "pay_info": tc.payInfo,
				}), platformPrivate)
				fmt.Fprintf(w, `{"code":0,"msg":"success","trade_no":"502V2-001","pay_type":%q,"pay_info":%q,"sign_type":"RSA","sign":%q}`, tc.payType, tc.payInfo, respSign)
			}))
			defer server.Close()

			cfg := pay502v2TestConfig(merchantPrivate, platformPublic)
			cfg["gateway_url"] = server.URL + "/xpay/epay"
			cfg["target_currency"] = "CNY"
			cfg["exchange_rate"] = "7"
			result, err := NewPay502V2Adapter().CreatePayment(context.Background(), cfg, paymentcontract.GatewayCreateInput{
				OrderNo:     "DJP502V2",
				Subject:     "商品 & 502",
				Amount:      money.FromDecimal(decimal.NewFromInt(10)),
				Currency:    "USD",
				ChannelType: tc.channelTyp,
				ClientIP:    "127.0.0.1",
				Extra:       jsonmap.JSON{"interaction_mode": tc.mode},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.ProviderRef != "502V2-001" || result.AmountSent != "70" || result.CurrencySent != "CNY" {
				t.Fatalf("unexpected result: %+v", result)
			}
			if tc.wantQR {
				if result.QRCodeURL != tc.payInfo || result.RedirectURL != "" {
					t.Fatalf("unexpected qr result: %+v", result)
				}
			} else if result.RedirectURL != tc.payInfo || result.QRCodeURL != "" {
				t.Fatalf("unexpected jump result: %+v", result)
			}
		})
	}
}

func TestPay502V2RejectsInvalidResponseSignature(t *testing.T) {
	merchantPrivate, _ := generateAdapterRSAKeyPair(t)
	_, platformPublic := generateAdapterRSAKeyPair(t)
	otherPrivate, _ := generateAdapterRSAKeyPair(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Signed by a key that does not match the configured platform public key.
		sign := signTestRSA(buildEpayAdapterSignContent(map[string]string{
			"code": "0", "msg": "success", "trade_no": "502V2-002", "pay_type": "qrcode", "pay_info": "https://pay.example/qr",
		}), otherPrivate)
		fmt.Fprintf(w, `{"code":0,"msg":"success","trade_no":"502V2-002","pay_type":"qrcode","pay_info":"https://pay.example/qr","sign_type":"RSA","sign":%q}`, sign)
	}))
	defer server.Close()

	cfg := pay502v2TestConfig(merchantPrivate, platformPublic)
	cfg["gateway_url"] = server.URL + "/xpay/epay"
	_, err := NewPay502V2Adapter().CreatePayment(context.Background(), cfg, paymentcontract.GatewayCreateInput{
		OrderNo: "DJP502V2", Subject: "x", Amount: money.FromDecimal(decimal.NewFromInt(1)),
		Currency: "CNY", ChannelType: "alipay", ClientIP: "127.0.0.1",
		Extra: jsonmap.JSON{"interaction_mode": "qr"},
	})
	if !errors.Is(err, paymentcontract.ErrGatewaySignatureInvalid) {
		t.Fatalf("expected signature invalid, got %v", err)
	}
}

func TestPay502V2ConfigAndModeValidation(t *testing.T) {
	merchantPrivate, _ := generateAdapterRSAKeyPair(t)
	_, platformPublic := generateAdapterRSAKeyPair(t)
	cfg := pay502v2TestConfig(merchantPrivate, platformPublic)
	a := NewPay502V2Adapter()

	for _, tc := range []struct {
		key   string
		value any
	}{{"epay_version", "v1"}, {"sign_type", "MD5"}, {"target_currency", "USD"}} {
		bad := pay502v2TestConfig(merchantPrivate, platformPublic)
		bad[tc.key] = tc.value
		if err := a.ValidateConfig(bad, "alipay"); !errors.Is(err, paymentcontract.ErrGatewayConfigInvalid) {
			t.Errorf("%s: %v", tc.key, err)
		}
	}
	for _, channelType := range []string{"qqpay", "usdt"} {
		if err := a.ValidateConfig(cfg, channelType); !errors.Is(err, paymentcontract.ErrGatewayUnsupportedChannel) {
			t.Errorf("%s: %v", channelType, err)
		}
	}
	// Unsupported interaction mode must fail without contacting the gateway.
	cfg["gateway_url"] = "http://127.0.0.1:1"
	_, err := a.CreatePayment(context.Background(), cfg, paymentcontract.GatewayCreateInput{
		OrderNo: "DJP502V2", Subject: "x", Amount: money.FromDecimal(decimal.NewFromInt(1)),
		Currency: "CNY", ChannelType: "alipay", ClientIP: "127.0.0.1",
		Extra: jsonmap.JSON{"interaction_mode": "wap"},
	})
	if !errors.Is(err, paymentcontract.ErrGatewayConfigInvalid) {
		t.Fatal(err)
	}
	// Non-CNY without conversion must fail before any request.
	_, err = a.CreatePayment(context.Background(), cfg, paymentcontract.GatewayCreateInput{
		OrderNo: "DJP502V2", Subject: "x", Amount: money.FromDecimal(decimal.NewFromInt(1)),
		Currency: "USD", ChannelType: "alipay", ClientIP: "127.0.0.1",
		Extra: jsonmap.JSON{"interaction_mode": "qr"},
	})
	if !errors.Is(err, paymentcontract.ErrGatewayConfigInvalid) {
		t.Fatal(err)
	}
}

func TestPay502V2Callback(t *testing.T) {
	merchantPrivate, _ := generateAdapterRSAKeyPair(t)
	platformPrivate, platformPublic := generateAdapterRSAKeyPair(t)
	cfg := pay502v2TestConfig(merchantPrivate, platformPublic)
	a := NewPay502V2Adapter().(paymentcontract.GatewayCallbackVerifier)

	for _, status := range []string{constants.EpayTradeStatusSuccess, "TRADE_FINISHED", "WAIT_BUYER_PAY"} {
		form := signEpayV2AdapterCallbackForm(map[string]string{
			"pid": "2002", "out_trade_no": "DJP502V2", "trade_no": "502V2-001",
			"trade_status": status, "money": "10.00",
		}, platformPrivate)
		result, err := a.VerifyCallback(cfg, form, nil)
		if err != nil {
			t.Fatal(err)
		}
		if (result.Status == constants.PaymentStatusSuccess) != (status == constants.EpayTradeStatusSuccess) {
			t.Fatalf("%s: %s", status, result.Status)
		}
	}

	// A callback signed by another key must be rejected.
	_, otherPublic := generateAdapterRSAKeyPair(t)
	otherCfg := pay502v2TestConfig(merchantPrivate, otherPublic)
	form := signEpayV2AdapterCallbackForm(map[string]string{
		"pid": "2002", "out_trade_no": "DJP502V2", "trade_no": "502V2-001",
		"trade_status": constants.EpayTradeStatusSuccess, "money": "10.00",
	}, platformPrivate)
	if _, err := a.VerifyCallback(otherCfg, form, nil); !errors.Is(err, paymentcontract.ErrGatewaySignatureInvalid) {
		t.Fatalf("expected signature invalid, got %v", err)
	}
}

func signEpayV2AdapterCallbackForm(params map[string]string, privateKeyPEM string) map[string][]string {
	form := make(map[string][]string, len(params)+2)
	for key, value := range params {
		form[key] = []string{value}
	}
	form["sign_type"] = []string{"RSA"}
	form["sign"] = []string{signTestRSA(buildEpayAdapterSignContent(params), privateKeyPEM)}
	return form
}

func signTestRSA(content, privateKeyPEM string) string {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}
	hashed := sha256.Sum256([]byte(content))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hashed[:])
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(signature)
}

func verifyTestRSASignature(content, signature, publicKeyPEM string) bool {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return false
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	hashed := sha256.Sum256([]byte(content))
	return rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, hashed[:], raw) == nil
}
