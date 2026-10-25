package epayadapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/dujiao-next/internal/constants"
	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
)

func pay502TestConfig() jsonmap.JSON {
	return jsonmap.JSON{"merchant_id": "1001", "merchant_key": "502-test-key", "notify_url": "https://shop.example/api/v1/payments/callback", "return_url": "https://shop.example/pay"}
}

func Test502CreateAPI(t *testing.T) {
	for _, field := range []string{"payurl", "qrcode", "urlscheme"} {
		t.Run(field, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/xpay/epay/mapi.php" {
					t.Errorf("unexpected endpoint %s %s", r.Method, r.URL.Path)
				}
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				params := map[string]string{}
				for k := range r.PostForm {
					params[k] = r.PostForm.Get(k)
				}
				if r.Form.Get("sign") != md5Hex(buildEpayAdapterSignContent(params)+"502-test-key") {
					t.Error("invalid request signature")
				}
				if r.Form.Get("pid") != "1001" || r.Form.Get("type") != "wxpay" || r.Form.Get("money") != "70" || r.Form.Get("out_trade_no") != "DJP502" || r.Form.Get("sign_type") != "MD5" {
					t.Errorf("unexpected request: %v", r.Form)
				}
				fmt.Fprintf(w, `{"code":1,"trade_no":"502001","%s":"https://pay.example/502"}`, field)
			}))
			defer server.Close()
			cfg := pay502TestConfig()
			cfg["gateway_url"] = server.URL + "/xpay/epay"
			cfg["target_currency"] = "CNY"
			cfg["exchange_rate"] = "7"
			result, err := NewPay502Adapter().CreatePayment(context.Background(), cfg, paymentcontract.GatewayCreateInput{OrderNo: "DJP502", Subject: "商品 & 502", Amount: money.FromDecimal(decimal.NewFromInt(10)), Currency: "USD", ChannelType: "wechat", ClientIP: "127.0.0.1", Extra: jsonmap.JSON{"interaction_mode": "qr"}})
			if err != nil {
				t.Fatal(err)
			}
			if result.ProviderRef != "502001" || result.AmountSent != "70" || result.CurrencySent != "CNY" {
				t.Fatalf("unexpected result: %+v", result)
			}
			if field == "qrcode" {
				if result.QRCodeURL != "https://pay.example/502" {
					t.Fatal(result)
				}
			} else if result.RedirectURL != "https://pay.example/502" {
				t.Fatal(result)
			}
		})
	}
}

func Test502RedirectAndConfig(t *testing.T) {
	a := NewPay502Adapter()
	cfg := pay502TestConfig()
	result, err := a.CreatePayment(context.Background(), cfg, paymentcontract.GatewayCreateInput{OrderNo: "DJP502", Subject: "测试 & 商品", Amount: money.FromDecimal(decimal.NewFromInt(10)), Currency: "CNY", ChannelType: "alipay", Extra: jsonmap.JSON{"interaction_mode": "redirect"}})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(result.RedirectURL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "pay.502k.cn" || u.Path != "/xpay/epay/submit.php" {
		t.Fatal(u)
	}
	params := map[string]string{}
	for k := range u.Query() {
		params[k] = u.Query().Get(k)
	}
	if u.Query().Get("sign") != md5Hex(buildEpayAdapterSignContent(params)+"502-test-key") {
		t.Fatal("invalid redirect signature")
	}
	if _, exists := cfg["gateway_url"]; exists {
		t.Fatal("mutated caller config")
	}
	for _, tc := range []struct {
		key   string
		value any
	}{{"epay_version", "v2"}, {"sign_type", "RSA"}, {"target_currency", "USD"}} {
		bad := pay502TestConfig()
		bad[tc.key] = tc.value
		if err := a.ValidateConfig(bad, "alipay"); !errors.Is(err, paymentcontract.ErrGatewayConfigInvalid) {
			t.Errorf("%s: %v", tc.key, err)
		}
	}
	if err := a.ValidateConfig(cfg, "qqpay"); !errors.Is(err, paymentcontract.ErrGatewayUnsupportedChannel) {
		t.Fatal(err)
	}
	// Invalid currency must fail without contacting the gateway.
	cfg["gateway_url"] = "http://127.0.0.1:1"
	_, err = a.CreatePayment(context.Background(), cfg, paymentcontract.GatewayCreateInput{Currency: "USD", ChannelType: "alipay"})
	if !errors.Is(err, paymentcontract.ErrGatewayConfigInvalid) {
		t.Fatal(err)
	}
}

func Test502CallbackStatus(t *testing.T) {
	a := NewPay502Adapter().(paymentcontract.GatewayCallbackVerifier)
	for _, status := range []string{"TRADE_SUCCESS", "TRADE_FINISHED", "WAIT_BUYER_PAY"} {
		form := signEpayV1AdapterCallbackForm(map[string]string{"pid": "1001", "out_trade_no": "DJP502", "trade_no": "502001", "trade_status": status, "money": "10.00"}, "502-test-key")
		result, err := a.VerifyCallback(pay502TestConfig(), form, nil)
		if err != nil {
			t.Fatal(err)
		}
		if (result.Status == constants.PaymentStatusSuccess) != (status == "TRADE_SUCCESS") {
			t.Fatalf("%s: %s", status, result.Status)
		}
	}
}
