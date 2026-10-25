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
func (s billingStoreStub) ListBillingByOrderIDs(ids []uint) ([]paymentdomain.Payment, error) {
	var payments []paymentdomain.Payment
	for _, payment := range s.payments {
		for _, id := range ids {
			if payment.OrderID == id {
				payments = append(payments, payment)
				break
			}
		}
	}
	return payments, s.err
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

func TestCanceledOrderBillingPreservesLastQuoteAndInvoiceTax(t *testing.T) {
	now := time.Now()
	expired := now.Add(-time.Minute)
	for _, policy := range []string{constants.PaymentFeePolicyCustomerSurcharge, constants.PaymentFeePolicyLegacyCustomerSurcharge, "", constants.PaymentFeePolicyMerchantAbsorbed} {
		t.Run(policy, func(t *testing.T) {
			root := &orderdomain.Order{ID: 1, Status: constants.OrderStatusCanceled, Currency: "CNY", InvoiceRequested: true,
				TotalAmount: billingMoney("123.60"), TaxAmount: billingMoney("3.60"), OnlinePaidAmount: billingMoney("123.60"), CanceledAt: &now,
				Children: []orderdomain.Order{
					{ID: 3, Status: constants.OrderStatusCanceled, TotalAmount: billingMoney("82.40"), TaxAmount: billingMoney("2.40")},
					{ID: 2, Status: constants.OrderStatusCanceled, TotalAmount: billingMoney("41.20"), TaxAmount: billingMoney("1.20")},
				},
			}
			payments := []paymentdomain.Payment{
				{ID: 10, OrderID: 1, Status: constants.PaymentStatusExpired, FeePolicy: constants.PaymentFeePolicyCustomerSurcharge, FeeAmount: billingMoney("9"), TaxAmount: billingMoney("0.27"), QRCode: "old", SupersededAt: &expired},
				{ID: 11, OrderID: 1, Status: constants.PaymentStatusExpired, FeePolicy: policy, FeeAmount: billingMoney("6"), TaxAmount: billingMoney("0.18"), QRCode: "last-quote", ExpiredAt: &expired},
				{ID: 12, OrderID: 1, Status: constants.PaymentStatusFailed, FeeAmount: billingMoney("99"), QRCode: "failed"},
				{ID: 13, OrderID: 1, Status: constants.PaymentStatusExpired, FeeAmount: billingMoney("99")},
				{ID: 14, OrderID: 1, Status: constants.PaymentStatusSuccess, FeeAmount: billingMoney("99"), ExceptionCode: constants.PaymentExceptionClosedOrderSucceeded},
				{ID: 15, OrderID: 2, Status: constants.PaymentStatusExpired, FeeAmount: billingMoney("99"), QRCode: "another-order"},
			}
			// Apply to pending then canceled, proving the fee and tax do not disappear.
			root.Status = constants.OrderStatusPendingPayment
			payments[1].Status = constants.PaymentStatusPending
			payments[1].ExpiredAt = nil
			applyOrderBilling(root, payments, now)
			before := orderpresenter.NewOrderDetail(root)
			root.Status = constants.OrderStatusCanceled
			payments[1].Status = constants.PaymentStatusExpired
			payments[1].ExpiredAt = &expired
			applyOrderBilling(root, payments, now)
			detail := orderpresenter.NewOrderDetail(root)
			wantFee, wantTax, wantPayable := "6.00", "3.78", "129.78"
			if policy == constants.PaymentFeePolicyMerchantAbsorbed {
				wantFee, wantTax, wantPayable = "0.00", "3.60", "123.60"
			}
			if detail.FeeAmount.StringFixed(2) != wantFee || detail.InvoiceTaxAmount.StringFixed(2) != wantTax || detail.PayableAmount.StringFixed(2) != wantPayable {
				t.Fatalf("canceled amounts %+v", detail)
			}
			if !detail.PayableAmount.Decimal.Equal(before.PayableAmount.Decimal) || !detail.InvoiceTaxAmount.Decimal.Equal(before.InvoiceTaxAmount.Decimal) {
				t.Fatal("cancel changed the recorded quote")
			}
			if !detail.PaidAmount.Decimal.IsZero() || detail.PaidAt != nil || !detail.RefundedAmount.Decimal.IsZero() {
				t.Fatal("unpaid quote was reported as payment or refund")
			}
			fees, taxes, totals := decimal.Zero, decimal.Zero, decimal.Zero
			for _, child := range detail.Children {
				fees = fees.Add(child.FeeAmount.Decimal)
				taxes = taxes.Add(child.InvoiceTaxAmount.Decimal)
				totals = totals.Add(child.PayableAmount.Decimal)
				if !child.PaidAmount.Decimal.IsZero() {
					t.Fatal("unpaid child reported paid")
				}
			}
			if fees.StringFixed(2) != wantFee || taxes.StringFixed(2) != wantTax || totals.StringFixed(2) != wantPayable {
				t.Fatalf("child sums fee=%s tax=%s total=%s", fees, taxes, totals)
			}
			if root.TotalAmount.StringFixed(2) != "123.60" || root.TaxAmount.StringFixed(2) != "3.60" {
				t.Fatal("settlement changed")
			}
		})
	}
}

func TestCanceledOrderBillingChoosesLatestUsableQuote(t *testing.T) {
	now := time.Now()
	root := &orderdomain.Order{ID: 1, Status: constants.OrderStatusCanceled, TotalAmount: billingMoney("100")}
	payments := []paymentdomain.Payment{
		{ID: 2, OrderID: 1, Status: constants.PaymentStatusExpired, FeePolicy: constants.PaymentFeePolicyMerchantAbsorbed, FeeAmount: billingMoney("3"), PayURL: "last"},
		{ID: 1, OrderID: 1, Status: constants.PaymentStatusExpired, FeePolicy: constants.PaymentFeePolicyCustomerSurcharge, FeeAmount: billingMoney("6"), PayURL: "older"},
	}
	applyOrderBilling(root, payments, now)
	if !root.CustomerFeeAmount.Decimal.IsZero() {
		t.Fatal("older customer fee replaced newer merchant-paid quote")
	}
	applyOrderBilling(root, nil, now)
	if !root.CustomerFeeAmount.Decimal.IsZero() || !root.PaymentTaxAmount.Decimal.IsZero() {
		t.Fatal("fee invented without a payment quote")
	}
}

func TestCanceledOrderBillingWithoutInvoiceAndDirectChild(t *testing.T) {
	now := time.Now()
	parentID := uint(1)
	child := orderdomain.Order{ID: 2, ParentID: &parentID, Status: constants.OrderStatusCanceled, TotalAmount: billingMoney("120")}
	root := &orderdomain.Order{ID: parentID, Status: constants.OrderStatusCanceled, TotalAmount: billingMoney("120"), Children: []orderdomain.Order{child}}
	store := billingStoreStub{root: root, payments: []paymentdomain.Payment{{ID: 1, OrderID: 1, Status: constants.PaymentStatusExpired, FeePolicy: constants.PaymentFeePolicyCustomerSurcharge, FeeAmount: billingMoney("6"), QRCode: "expired-quote", ExpiredAt: &now}}}
	got, err := (orderBillingReader{orders: store, payments: store}).enrich(&child, nil)
	if err != nil {
		t.Fatal(err)
	}
	detail := orderpresenter.NewOrderDetail(got)
	if detail.FeeAmount.StringFixed(2) != "6.00" || !detail.InvoiceTaxAmount.Decimal.IsZero() || detail.PayableAmount.StringFixed(2) != "126.00" || !detail.PaidAmount.Decimal.IsZero() {
		t.Fatalf("non-invoice child billing %+v", detail)
	}
}

type billingBatchSpy struct {
	billingStoreStub
	batchCalls  int
	singleCalls int
}

func (s *billingBatchSpy) ListByOrderID(id uint) ([]paymentdomain.Payment, error) {
	s.singleCalls++
	return s.billingStoreStub.ListByOrderID(id)
}

func (s *billingBatchSpy) ListBillingByOrderIDs(ids []uint) ([]paymentdomain.Payment, error) {
	s.batchCalls++
	return s.billingStoreStub.ListBillingByOrderIDs(ids)
}

func TestOrderListBillingMatchesDetailsInOneBatch(t *testing.T) {
	now := time.Now()
	orders := []orderdomain.Order{
		{ID: 1, PaidAt: &now, TotalAmount: billingMoney("123.60"), TaxAmount: billingMoney("3.60"), Children: []orderdomain.Order{{ID: 11, PaidAt: &now, TotalAmount: billingMoney("61.80")}, {ID: 12, PaidAt: &now, TotalAmount: billingMoney("61.80")}}},
		{ID: 2, Status: constants.OrderStatusCanceled, TotalAmount: billingMoney("123.60"), TaxAmount: billingMoney("3.60")},
	}
	store := &billingBatchSpy{billingStoreStub: billingStoreStub{payments: []paymentdomain.Payment{
		{ID: 1, OrderID: 1, Status: constants.PaymentStatusSuccess, FeeAmount: billingMoney("6"), TaxAmount: billingMoney("0.18")},
		{ID: 2, OrderID: 2, Status: constants.PaymentStatusExpired, FeeAmount: billingMoney("6"), TaxAmount: billingMoney("0.18"), QRCode: "quote"},
		{ID: 3, OrderID: 99, Status: constants.PaymentStatusSuccess, FeeAmount: billingMoney("999")},
	}}}
	reader := orderBillingReader{payments: store}
	got, total, err := reader.enrichList(orders, 42, nil)
	if err != nil || total != 42 || len(got) != 2 || store.batchCalls != 1 || store.singleCalls != 0 {
		t.Fatalf("batch result total=%d err=%v batch=%d single=%d", total, err, store.batchCalls, store.singleCalls)
	}
	for i := range got {
		summary := orderpresenter.NewOrderSummary(&got[i])
		detail := orderpresenter.NewOrderDetail(&got[i])
		if summary.PayableAmount.StringFixed(2) != "129.78" || !summary.PayableAmount.Equal(detail.PayableAmount.Decimal) || !summary.PaidAmount.Equal(detail.PaidAmount.Decimal) || summary.InvoiceTaxAmount.StringFixed(2) != "3.78" {
			t.Fatalf("list/detail amount mismatch: %+v", summary)
		}
		if i == 1 && !summary.PaidAmount.IsZero() {
			t.Fatal("canceled order must have zero paid amount")
		}
	}
	children := orderpresenter.NewOrderSummary(&got[0]).Children
	if len(children) != 2 || children[0].PaidAmount.StringFixed(2) != "64.89" || children[1].PaidAmount.StringFixed(2) != "64.89" {
		t.Fatalf("child totals: %+v", children)
	}
	store.err = errors.New("payment read failed")
	if _, _, err := reader.enrichList(orders, 42, nil); err == nil {
		t.Fatal("failed payment lookup must not return incomplete totals")
	}
	calls := store.batchCalls
	if _, _, err := reader.enrichList(nil, 0, nil); err != nil || calls != store.batchCalls {
		t.Fatal("empty page queried payments")
	}
}
