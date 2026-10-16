package application

import (
	"context"
	"errors"
	cardsecretdomain "github.com/dujiao-next/internal/modules/cardsecret/domain"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	coupondomain "github.com/dujiao-next/internal/modules/coupon/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
	"testing"
	"time"

	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"

	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/modules/payment/infrastructure/gateway/provider"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

// recordingCapturer 捕获传入 ValidateConfig 的第二参数,用于回归测试。
type recordingCapturer struct {
	receivedMode string
}

func (r *recordingCapturer) Type() string { return "test_capturer" }

func (r *recordingCapturer) ValidateConfig(_ jsonmap.JSON, mode string) error {
	r.receivedMode = mode
	return errors.New("stop-test")
}

func (r *recordingCapturer) CreatePayment(_ context.Context, _ jsonmap.JSON, _ paymentcontract.GatewayCreateInput) (*paymentcontract.GatewayCreateResult, error) {
	return nil, errors.New("not-implemented")
}

func (r *recordingCapturer) QueryPayment(_ context.Context, _ jsonmap.JSON, _ string) (*paymentcontract.GatewayQueryResult, error) {
	return nil, errors.New("not-implemented")
}

// 防止重新引入 stripe/wechat capture 全部失败的回归:
// captureViaRegistry 必须把 channel.InteractionMode(而非 channel.ChannelType)
// 传给 Capturer.ValidateConfig,否则 stripe/wechat adapter 会判定 mode 非法。
func TestCaptureViaRegistry_PassesInteractionModeNotChannelType(t *testing.T) {
	cap := &recordingCapturer{}
	reg := provider.NewRegistry()
	reg.Register("test_provider", "test_channel", cap)

	svc := &PaymentService{paymentProviderRegistry: reg}

	channel := &paymentdomain.PaymentChannel{
		ID:              1,
		ProviderType:    "test_provider",
		ChannelType:     "test_channel",
		InteractionMode: constants.PaymentInteractionQR,
	}
	payment := &paymentdomain.Payment{
		ID:          1,
		ChannelID:   1,
		ProviderRef: "ref-123",
	}

	_, _ = svc.captureViaRegistry(CapturePaymentInput{}, payment, channel)

	if cap.receivedMode != channel.InteractionMode {
		t.Fatalf("ValidateConfig received mode=%q, want %q (channel.InteractionMode)",
			cap.receivedMode, channel.InteractionMode)
	}
	if cap.receivedMode == channel.ChannelType {
		t.Fatalf("regression: ValidateConfig received channel.ChannelType=%q", cap.receivedMode)
	}
}

func TestRecoverClosedOrder(t *testing.T) {
	for _, scenario := range []string{"success", "insufficient_stock", "underpaid", "already_refunded", "not_closed", "second_child_unavailable", "upstream", "upstream_queue_disabled", "auto", "coupon", "coupon_exhausted"} {
		t.Run(scenario, func(t *testing.T) {
			svc, db := setupPaymentServiceWalletTest(t)
			sqlDB, _ := db.DB()
			sqlDB.SetMaxOpenConns(1)
			procurement := &recoveryProcurement{}
			if scenario == "upstream" || scenario == "auto" {
				svc.queue = &affiliateRetryQueue{}
				svc.procurementSvc = procurement
			}
			now := time.Now()
			amount := money.FromDecimal(decimal.NewFromInt(120))
			product := productdomain.Product{Slug: "recover", TitleJSON: jsonmap.JSON{"zh-CN": "Recovery"}, IsActive: true, ManualStockTotal: 10}
			if err := db.Create(&product).Error; err != nil {
				t.Fatal(err)
			}
			sku := productdomain.ProductSKU{ProductID: product.ID, SKUCode: "recover", IsActive: true, ManualStockTotal: 10}
			if scenario == "insufficient_stock" {
				sku.ManualStockTotal = 0
			}
			if err := db.Create(&sku).Error; err != nil {
				t.Fatal(err)
			}
			parent := orderdomain.Order{OrderNo: "RECOVER", Status: constants.OrderStatusCanceled, Currency: "CNY", TotalAmount: amount, CanceledAt: &now, ExpiresAt: &now}
			if scenario == "already_refunded" {
				parent.RefundedAmount = amount
			}
			if scenario == "not_closed" {
				parent.Status = constants.OrderStatusPendingPayment
			}
			if scenario == "coupon" || scenario == "coupon_exhausted" {
				if err := db.AutoMigrate(&coupondomain.Coupon{}, &coupondomain.CouponUsage{}); err != nil {
					t.Fatal(err)
				}
				coupon := coupondomain.Coupon{Code: "recovery", Type: "fixed", ScopeType: "product", UsageLimit: 1}
				if scenario == "coupon_exhausted" {
					coupon.UsedCount = 1
				}
				if err := db.Create(&coupon).Error; err != nil {
					t.Fatal(err)
				}
				parent.CouponID = &coupon.ID
			}

			if err := db.Create(&parent).Error; err != nil {
				t.Fatal(err)
			}
			child := orderdomain.Order{ParentID: &parent.ID, OrderNo: "RECOVER-01", Status: constants.OrderStatusCanceled, Currency: "CNY", TotalAmount: amount, CanceledAt: &now, ExpiresAt: &now,
				Items: []orderdomain.OrderItem{{TitleJSON: jsonmap.JSON{"zh-CN": "Recovery"}, ProductID: product.ID, SKUID: sku.ID, Quantity: 1, FulfillmentType: constants.FulfillmentTypeManual}}}
			if scenario == "upstream" || scenario == "upstream_queue_disabled" {
				child.Items[0].FulfillmentType = constants.FulfillmentTypeUpstream
			}
			if scenario == "auto" {
				child.Items[0].FulfillmentType = constants.FulfillmentTypeAuto
				if err := db.AutoMigrate(&cardsecretdomain.Secret{}); err != nil {
					t.Fatal(err)
				}
				if err := db.Create(&cardsecretdomain.Secret{ProductID: product.ID, SKUID: sku.ID, Status: "available", Secret: "test-secret"}).Error; err != nil {
					t.Fatal(err)
				}
			}

			if err := db.Create(&child).Error; err != nil {
				t.Fatal(err)
			}
			if scenario == "second_child_unavailable" {
				missing := orderdomain.Order{ParentID: &parent.ID, OrderNo: "RECOVER-02", Status: constants.OrderStatusCanceled, Items: []orderdomain.OrderItem{{TitleJSON: jsonmap.JSON{"zh-CN": "Recovery"}, ProductID: 9999, Quantity: 1, FulfillmentType: constants.FulfillmentTypeManual}}}
				if err := db.Create(&missing).Error; err != nil {
					t.Fatal(err)
				}
			}
			payment := paymentdomain.Payment{OrderID: parent.ID, Status: constants.PaymentStatusSuccess, ExceptionCode: constants.PaymentExceptionClosedOrderSucceeded, Amount: money.FromDecimal(decimal.NewFromInt(126)), FeeAmount: money.FromDecimal(decimal.NewFromInt(6)), FeePolicy: "customer_surcharge", Currency: "CNY", PaidAt: &now}
			if scenario == "underpaid" {
				payment.Amount = money.FromDecimal(decimal.NewFromInt(100))
			}
			if err := db.Create(&payment).Error; err != nil {
				t.Fatal(err)
			}
			_, err := svc.RecoverClosedOrder(payment.ID, 1)
			if scenario != "success" && scenario != "upstream" && scenario != "auto" && scenario != "coupon" {
				if err == nil {
					t.Fatal("expected recovery rejection")
				}
				var stored orderdomain.Order
				db.First(&stored, parent.ID)
				if stored.Status != parent.Status || stored.PaidAt != nil {
					t.Fatal("failed recovery changed order")
				}
				var stock productdomain.ProductSKU
				db.First(&stock, sku.ID)
				if stock.ManualStockLocked != 0 || stock.ManualStockSold != 0 {
					t.Fatal("failed recovery leaked stock reservation")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			// Repeated operator requests must not consume inventory twice.
			if _, err := svc.RecoverClosedOrder(payment.ID, 1); err != nil {
				t.Fatal(err)
			}
			for _, id := range []uint{parent.ID, child.ID} {
				var stored orderdomain.Order
				if err := db.First(&stored, id).Error; err != nil {
					t.Fatal(err)
				}
				expectedStatus := constants.OrderStatusFulfilling
				if scenario == "auto" {
					expectedStatus = constants.OrderStatusPaid
				}
				if stored.Status != expectedStatus || stored.PaidAt == nil || stored.CanceledAt != nil || stored.ExpiresAt != nil {
					t.Fatalf("invalid recovered order: %s", stored.Status)
				}
			}
			var stock productdomain.ProductSKU
			db.First(&stock, sku.ID)
			expectedSold := 1
			if scenario == "auto" {
				expectedSold = 0
				var secret cardsecretdomain.Secret
				if err := db.First(&secret).Error; err != nil {
					t.Fatal(err)
				}
				if secret.Status != "reserved" || secret.OrderID == nil || *secret.OrderID != child.ID {
					t.Fatal("card was not reserved for recovery")
				}
			}
			if scenario == "coupon" {
				var coupon coupondomain.Coupon
				db.First(&coupon, *parent.CouponID)
				var count int64
				db.Model(&coupondomain.CouponUsage{}).Where("order_id = ?", parent.ID).Count(&count)
				if coupon.UsedCount != 1 || count != 1 {
					t.Fatal("coupon usage not restored exactly once")
				}
			}
			if scenario == "upstream" {
				expectedSold = 0
				if len(procurement.created) != 1 || procurement.created[0] != parent.ID {
					t.Fatal("procurement not triggered exactly once")
				}
			}
			if stock.ManualStockLocked != 0 || stock.ManualStockSold != expectedSold {
				t.Fatalf("stock changed more than once: %+v", stock)
			}
			var stored paymentdomain.Payment
			db.First(&stored, payment.ID)
			if stored.ExceptionCode != "" || stored.ProviderPayload["closed_order_recovered_by"] == nil || !stored.Amount.Decimal.Equal(payment.Amount.Decimal) {
				t.Fatal("missing audit or changed payment")
			}
		})
	}
}

type recoveryProcurement struct{ created []uint }

func (p *recoveryProcurement) CreateForOrder(id uint) error {
	p.created = append(p.created, id)
	return nil
}
