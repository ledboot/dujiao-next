package orderwiring

import (
	"encoding/json"
	"errors"
	"github.com/dujiao-next/internal/constants"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	orderpresenter "github.com/dujiao-next/internal/modules/order/transport/presenter"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
	"testing"
	"time"
)

func billingMoney(s string) money.Amount { return money.FromDecimal(decimal.RequireFromString(s)) }

func TestOrderBillingIncludesFeeAndTaxWithoutChangingSettlement(t *testing.T) {
	now := time.Now()
	root := &orderdomain.Order{ID: 1, Currency: "CNY", TotalAmount: billingMoney("123.60"), TaxAmount: billingMoney("3.60"), OnlinePaidAmount: billingMoney("123.60"), PaidAt: &now,
		Children: []orderdomain.Order{
			{ID: 3, TotalAmount: billingMoney("82.40"), TaxAmount: billingMoney("2.40"), OnlinePaidAmount: billingMoney("82.40"), PaidAt: &now},
			{ID: 2, TotalAmount: billingMoney("41.20"), TaxAmount: billingMoney("1.20"), OnlinePaidAmount: billingMoney("41.20"), PaidAt: &now},
		}}
	payments := []paymentdomain.Payment{{ID: 10, OrderID: 1, Status: constants.PaymentStatusSuccess, Amount: billingMoney("129.78"), FeeAmount: billingMoney("6.00"), TaxAmount: billingMoney("0.18"), FeePolicy: constants.PaymentFeePolicyCustomerSurcharge},
		{ID: 11, OrderID: 1, Status: constants.PaymentStatusSuccess, FeeAmount: billingMoney("99"), ExceptionCode: "duplicate"},
		{ID: 12, OrderID: 1, Status: constants.PaymentStatusPending, FeeAmount: billingMoney("88")},
	}
	applyOrderBilling(root, payments, now)
	raw, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	var adminView struct {
		Fee      money.Amount `json:"fee_amount"`
		FeeTax   money.Amount `json:"payment_tax_amount"`
		Children []struct {
			Fee money.Amount `json:"fee_amount"`
		} `json:"children"`
	}
	if err := json.Unmarshal(raw, &adminView); err != nil {
		t.Fatal(err)
	}
	if adminView.Fee.StringFixed(2) != "6.00" || adminView.FeeTax.StringFixed(2) != "0.18" || len(adminView.Children) != 2 {
		t.Fatalf("admin billing missing: %s", raw)
	}
	detail := orderpresenter.NewOrderDetail(root)
	if detail.PaidAmount.StringFixed(2) != "129.78" || detail.InvoiceTaxAmount.StringFixed(2) != "3.78" || detail.FeeAmount.StringFixed(2) != "6.00" || detail.OnlinePayableAmount.StringFixed(2) != "129.78" {
		t.Fatalf("parent billing %+v", detail)
	}
	fee, tax, paid := decimal.Zero, decimal.Zero, decimal.Zero
	for _, child := range detail.Children {
		fee = fee.Add(child.FeeAmount.Decimal)
		tax = tax.Add(child.InvoiceTaxAmount.Decimal)
		paid = paid.Add(child.PaidAmount.Decimal)
	}
	if fee.StringFixed(2) != "6.00" || tax.StringFixed(2) != "3.78" || paid.StringFixed(2) != "129.78" {
		t.Fatalf("children fee=%s tax=%s paid=%s", fee, tax, paid)
	}
	if root.TotalAmount.StringFixed(2) != "123.60" || root.TaxAmount.StringFixed(2) != "3.60" || root.OnlinePaidAmount.StringFixed(2) != "123.60" {
		t.Fatal("read model changed settlement bases")
	}
}

func TestOrderBillingPendingSelectionAndMerchantFees(t *testing.T) {
	now := time.Now()
	expired := now.Add(-time.Minute)
	future := now.Add(time.Minute)
	for _, policy := range []string{constants.PaymentFeePolicyCustomerSurcharge, constants.PaymentFeePolicyMerchantAbsorbed, ""} {
		t.Run(policy, func(t *testing.T) {
			root := &orderdomain.Order{ID: 1, Status: constants.OrderStatusPendingPayment, TotalAmount: billingMoney("123.60"), TaxAmount: billingMoney("3.60")}
			payments := []paymentdomain.Payment{
				{ID: 1, OrderID: 1, Status: constants.PaymentStatusPending, FeePolicy: policy, FeeAmount: billingMoney("6"), TaxAmount: billingMoney("0.18"), ExpiredAt: &future},
				{ID: 2, OrderID: 1, Status: constants.PaymentStatusPending, FeeAmount: billingMoney("99"), SupersededAt: &now},
				{ID: 3, OrderID: 1, Status: constants.PaymentStatusPending, FeeAmount: billingMoney("99"), ExpiredAt: &expired},
				{ID: 4, OrderID: 2, Status: constants.PaymentStatusSuccess, FeeAmount: billingMoney("99")},
			}
			applyOrderBilling(root, payments, now)
			detail := orderpresenter.NewOrderDetail(root)
			want := "129.78"
			if policy == constants.PaymentFeePolicyMerchantAbsorbed {
				want = "123.60"
			}
			if detail.PayableAmount.StringFixed(2) != want || !detail.PaidAmount.Decimal.IsZero() {
				t.Fatalf("pending billing %+v", detail)
			}
		})
	}
}

type billingStoreStub struct {
	root     *orderdomain.Order
	payments []paymentdomain.Payment
	err      error
}

func (s billingStoreStub) GetByID(uint) (*orderdomain.Order, error) { return s.root, s.err }
func (s billingStoreStub) ListByOrderID(uint) ([]paymentdomain.Payment, error) {
	return s.payments, s.err
}
func TestOrderBillingDirectChildUsesParentAllocationAndPropagatesErrors(t *testing.T) {
	now := time.Now()
	parentID := uint(1)
	child := orderdomain.Order{ID: 2, ParentID: &parentID, TotalAmount: billingMoney("123.60"), TaxAmount: billingMoney("3.60"), PaidAt: &now}
	root := &orderdomain.Order{ID: 1, TotalAmount: billingMoney("123.60"), PaidAt: &now, Children: []orderdomain.Order{child}}
	store := billingStoreStub{root: root, payments: []paymentdomain.Payment{{ID: 1, OrderID: 1, Status: constants.PaymentStatusSuccess, FeePolicy: constants.PaymentFeePolicyCustomerSurcharge, FeeAmount: billingMoney("6"), TaxAmount: billingMoney("0.18")}}}
	got, err := (orderBillingReader{orders: store, payments: store}).enrich(&child, nil)
	if err != nil || orderpresenter.NewOrderDetail(got).PaidAmount.StringFixed(2) != "129.78" {
		t.Fatalf("direct child %+v, %v", got, err)
	}
	store.err = errors.New("database unavailable")
	if _, err := (orderBillingReader{orders: store, payments: store}).enrich(&child, nil); err == nil {
		t.Fatal("missing database error")
	}
}

func TestOrderBillingChildAllocationAbsorbsRounding(t *testing.T) {
	now := time.Now()
	root := &orderdomain.Order{ID: 1, TotalAmount: billingMoney("1.00"), Children: []orderdomain.Order{{ID: 2, TotalAmount: billingMoney("0.33")}, {ID: 3, TotalAmount: billingMoney("0.33")}, {ID: 4, TotalAmount: billingMoney("0.34")}}}
	applyOrderBilling(root, []paymentdomain.Payment{{OrderID: 1, Status: constants.PaymentStatusSuccess, FeePolicy: constants.PaymentFeePolicyCustomerSurcharge, FeeAmount: billingMoney("0.01"), TaxAmount: billingMoney("0.01")}}, now)
	fee, tax := decimal.Zero, decimal.Zero
	for _, child := range root.Children {
		fee = fee.Add(child.CustomerFeeAmount.Decimal)
		tax = tax.Add(child.PaymentTaxAmount.Decimal)
	}
	if fee.StringFixed(2) != "0.01" || tax.StringFixed(2) != "0.01" {
		t.Fatalf("rounding allocation fee=%s tax=%s", fee, tax)
	}
}
