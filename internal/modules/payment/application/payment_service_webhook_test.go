package application

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

func TestSupportsBlindWebhookCandidateMatching(t *testing.T) {
	cases := []struct {
		channelType string
		want        bool
	}{
		{constants.PaymentChannelTypeWechat, true},
		{constants.PaymentChannelTypeStripe, true},
		{constants.PaymentChannelTypePaypal, false},
		{constants.PaymentChannelTypeAlipay, false},
		{"", false},
		{"  WECHAT  ", true},
	}
	for _, tc := range cases {
		if got := supportsBlindWebhookCandidateMatching(tc.channelType); got != tc.want {
			t.Fatalf("channelType=%q: got %v, want %v", tc.channelType, got, tc.want)
		}
	}
}

// TestHandleWechatWebhookFallbackNoCandidate 验证 channel_id 为 0 且没有任何 active wechat
// channel 时,wechat webhook 返回 ErrPaymentChannelNotFound(而非 ErrPaymentInvalid);
// 这是相对 67bbc47 重构后回归行为(硬性拒绝 channel_id==0)的关键修复。
func TestHandleWechatWebhookFallbackNoCandidate(t *testing.T) {
	svc, _ := setupPaymentServiceWalletTest(t)

	_, _, err := svc.HandleWechatWebhook(WebhookCallbackInput{
		ChannelID: 0,
		Body:      []byte(`{"resource":{}}`),
		Headers:   map[string]string{},
	})
	if !errors.Is(err, ErrPaymentChannelNotFound) {
		t.Fatalf("expected ErrPaymentChannelNotFound for empty wechat candidate list, got: %v", err)
	}
}

// TestHandlePaypalWebhookRequiresChannelID 验证 paypal webhook 必须带 channel_id,
// 不具备盲匹配能力;channel_id==0 直接走 ErrPaymentInvalid。
func TestHandlePaypalWebhookRequiresChannelID(t *testing.T) {
	svc, _ := setupPaymentServiceWalletTest(t)

	_, _, err := svc.HandlePaypalWebhook(WebhookCallbackInput{
		ChannelID: 0,
		Body:      []byte(`{}`),
		Headers:   map[string]string{},
	})
	if !errors.Is(err, ErrPaymentInvalid) {
		t.Fatalf("expected ErrPaymentInvalid for paypal without channel_id, got: %v", err)
	}
}

func TestWebhookSharedSecretMatchesPaymentChannel(t *testing.T) {
	for _, provider := range []string{constants.PaymentProviderDujiaoPay, constants.PaymentChannelTypeStripe} {
		for _, scenario := range []string{"paid", "bad-signature", "wrong-channel", "unknown-order", "database-error"} {
			t.Run(provider+"/"+scenario, func(t *testing.T) {
				svc, db := setupPaymentServiceWalletTest(t)
				channel, payment, recharge := createDujiaoPayWalletWebhookFixture(t, db)
				body := []byte(fmt.Sprintf(`{"event_id":"evt-paid","event_type":"order.paid","data":{"order_id":%q,"merchant_order_id":%q,"fiat_currency":"CNY","fiat_amount":"88.00"}}`, payment.ProviderRef, payment.GatewayOrderNo))
				headers := signDujiaoPayWebhook(body, "test-dujiaopay-webhook-secret")
				handle := svc.HandleDujiaoPayWebhook
				if provider == constants.PaymentChannelTypeStripe {
					channel.ProviderType = constants.PaymentProviderOfficial
					channel.ChannelType = constants.PaymentChannelTypeStripe
					channel.ConfigJSON = jsonmap.JSON{"secret_key": "sk_test", "webhook_secret": "whsec_test", "success_url": "https://example.test/pay", "cancel_url": "https://example.test/pay"}
					if err := db.Save(channel).Error; err != nil {
						t.Fatal(err)
					}
					if err := db.Model(payment).Updates(map[string]interface{}{"provider_type": channel.ProviderType, "channel_type": channel.ChannelType}).Error; err != nil {
						t.Fatal(err)
					}
					body = []byte(fmt.Sprintf(`{"id":"evt-paid","type":"checkout.session.completed","data":{"object":{"object":"checkout.session","id":%q,"metadata":{"order_no":%q},"currency":"cny","amount_total":8800,"payment_status":"paid","status":"complete"}}}`, payment.ProviderRef, payment.GatewayOrderNo))
					timestamp := strconv.FormatInt(time.Now().Unix(), 10)
					mac := hmac.New(sha256.New, []byte("whsec_test"))
					_, _ = mac.Write([]byte(timestamp + "."))
					_, _ = mac.Write(body)
					headers = map[string]string{"Stripe-Signature": "t=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))}
					handle = svc.HandleStripeWebhook
				}
				// 同一密钥的无关渠道优先排序，旧实现会在这里误匹配并吞掉通知。
				decoy := *channel
				decoy.ID = 0
				decoy.Name = "shared secret decoy"
				decoy.SortOrder = 100
				if err := db.Create(&decoy).Error; err != nil {
					t.Fatal(err)
				}
				input := WebhookCallbackInput{Body: body, Headers: headers}
				switch scenario {
				case "bad-signature":
					input.Body = append(body, ' ')
				case "wrong-channel":
					input.ChannelID = decoy.ID
				case "unknown-order":
					if err := db.Model(payment).Updates(map[string]interface{}{"gateway_order_no": "different-order", "provider_ref": "different-ref"}).Error; err != nil {
						t.Fatal(err)
					}
				case "database-error":
					svc.paymentRepo = &webhookLookupErrorStore{Store: svc.paymentRepo}
				}
				updated, status, err := handle(input)
				if scenario == "paid" {
					if err != nil || updated == nil || updated.ID != payment.ID || status != constants.PaymentStatusSuccess {
						t.Fatalf("did not match real channel: payment=%+v status=%s err=%v", updated, status, err)
					}
					// 重放成功事件必须只入账一次。
					if _, _, err := handle(input); err != nil {
						t.Fatal(err)
					}
					assertWalletRechargeSuccessState(t, db, payment.ID, recharge.ID, recharge.UserID, recharge.Amount.Decimal)
					return
				}
				if scenario == "bad-signature" && err == nil {
					t.Fatal("accepted invalid signature")
				}
				if scenario == "database-error" && !errors.Is(err, ErrPaymentUpdateFailed) {
					t.Fatalf("database failure acknowledged: %v", err)
				}
				if (scenario == "wrong-channel" || scenario == "unknown-order") && (err != nil || updated != nil) {
					t.Fatalf("unknown payment behavior changed: %+v %v", updated, err)
				}
				var stored paymentdomain.Payment
				if err := db.First(&stored, payment.ID).Error; err != nil {
					t.Fatal(err)
				}
				if stored.Status != constants.PaymentStatusPending {
					t.Fatalf("unexpected payment update: %+v", stored)
				}
			})
		}
	}
}

type webhookLookupErrorStore struct{ paymentcontract.Store }

func (s *webhookLookupErrorStore) GetByGatewayOrderNo(string) (*paymentdomain.Payment, error) {
	return nil, errors.New("database unavailable")
}
