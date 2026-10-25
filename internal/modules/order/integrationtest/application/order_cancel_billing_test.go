package application_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	productgormstore "github.com/dujiao-next/internal/modules/catalog/product/store/gormstore"
	coupongormstore "github.com/dujiao-next/internal/modules/coupon/infrastructure/gormstore"
	. "github.com/dujiao-next/internal/modules/order/application"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	ordergormstore "github.com/dujiao-next/internal/modules/order/infrastructure/gormstore"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
)

func TestCancelOrderPreservesInvoiceAndPaymentFeeSnapshots(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		t.Run(fmt.Sprintf("automatic=%v", automatic), func(t *testing.T) {
			db := setupCancelPaymentTestDB(t, "invoice_fee_snapshot")
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			sqlDB.SetMaxOpenConns(1)
			now := time.Now()
			order := newPendingOrderForCancel("CANCEL-INVOICE-FEE", 42, nil, now)
			order.InvoiceRequested = true
			order.TotalAmount = money.FromDecimal(decimal.RequireFromString("123.60"))
			order.TaxAmount = money.FromDecimal(decimal.RequireFromString("3.60"))
			order.OnlinePaidAmount = order.TotalAmount
			expiry := now.Add(time.Minute)
			if automatic {
				expiry = now.Add(-time.Minute)
			}
			order.ExpiresAt = &expiry
			if err := db.Create(order).Error; err != nil {
				t.Fatal(err)
			}
			payment := newPaymentForOrder(order.ID, constants.PaymentStatusPending, now)
			payment.Amount = money.FromDecimal(decimal.RequireFromString("129.78"))
			payment.FeeAmount = money.FromDecimal(decimal.RequireFromString("6.00"))
			payment.TaxAmount = money.FromDecimal(decimal.RequireFromString("0.18"))
			payment.FeePolicy = constants.PaymentFeePolicyCustomerSurcharge
			payment.QRCode = "invoice-payment"
			if err := db.Create(payment).Error; err != nil {
				t.Fatal(err)
			}
			svc := NewOrderService(OrderServiceOptions{
				OrderStore:   ordergormstore.New(db, "test-guest-credential-secret-with-32-bytes"),
				ProductStore: productgormstore.NewProductStore(db), ProductSKUStore: productgormstore.NewSKUStore(db),
				CouponStore: coupongormstore.New(db), CouponUsageStore: coupongormstore.NewUsageStore(db),
			})
			if automatic {
				_, err = svc.CancelExpiredOrder(order.ID)
			} else {
				_, err = svc.CancelOrder(order.ID, 42)
			}
			if err != nil {
				t.Fatal(err)
			}
			var storedOrder orderdomain.Order
			if err := db.First(&storedOrder, order.ID).Error; err != nil {
				t.Fatal(err)
			}
			var storedPayment paymentdomain.Payment
			if err := db.First(&storedPayment, payment.ID).Error; err != nil {
				t.Fatal(err)
			}
			if storedOrder.Status != constants.OrderStatusCanceled || storedOrder.TaxAmount.StringFixed(2) != "3.60" || storedOrder.TotalAmount.StringFixed(2) != "123.60" || storedOrder.PaidAt != nil {
				t.Fatalf("order snapshot changed: %+v", storedOrder)
			}
			if storedPayment.Status != constants.PaymentStatusExpired || storedPayment.Amount.StringFixed(2) != "129.78" || storedPayment.FeeAmount.StringFixed(2) != "6.00" || storedPayment.TaxAmount.StringFixed(2) != "0.18" {
				t.Fatalf("payment snapshot changed: %+v", storedPayment)
			}
		})
	}
}
