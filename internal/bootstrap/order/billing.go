package orderwiring

import (
	"sort"
	"time"

	"github.com/dujiao-next/internal/constants"
	orderapp "github.com/dujiao-next/internal/modules/order/application"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
)

// Detail billing is a read model. It must never change order settlement or refund bases.
type orderBillingReader struct {
	orders interface {
		GetByID(uint) (*orderdomain.Order, error)
	}
	payments interface {
		ListByOrderID(uint) ([]paymentdomain.Payment, error)
	}
}

func (r orderBillingReader) enrich(order *orderdomain.Order, queryErr error) (*orderdomain.Order, error) {
	if queryErr != nil || order == nil || r.payments == nil {
		return order, mapOrderTransportError(queryErr)
	}
	root := order
	if order.ParentID != nil {
		var err error
		root, err = r.orders.GetByID(*order.ParentID)
		if err != nil {
			return nil, err
		}
		if root == nil {
			return nil, orderapp.ErrOrderFetchFailed
		}
	}
	payments, err := r.payments.ListByOrderID(root.ID)
	if err != nil {
		return nil, err
	}
	applyOrderBilling(root, payments, time.Now())
	if root.ID == order.ID {
		return root, nil
	}
	for _, child := range root.Children {
		if child.ID == order.ID {
			order.CustomerFeeAmount = child.CustomerFeeAmount
			order.PaymentTaxAmount = child.PaymentTaxAmount
			return order, nil
		}
	}
	return nil, orderapp.ErrOrderFetchFailed
}

func applyOrderBilling(order *orderdomain.Order, payments []paymentdomain.Payment, now time.Time) {
	fee, tax := decimal.Zero, decimal.Zero
	var pending, canceled *paymentdomain.Payment
	hasSuccessfulPayment := false
	for i := range payments {
		payment := &payments[i]
		if payment.OrderID != order.ID || payment.ExceptionCode != "" {
			continue
		}
		if payment.Status == constants.PaymentStatusSuccess {
			hasSuccessfulPayment = true
			if payment.FeePolicy == constants.PaymentFeePolicyCustomerSurcharge || payment.FeePolicy == constants.PaymentFeePolicyLegacyCustomerSurcharge || payment.FeePolicy == "" {
				fee = fee.Add(payment.FeeAmount.Decimal)
				tax = tax.Add(payment.TaxAmount.Decimal)
			}
		} else if order.PaidAt == nil && order.Status == constants.OrderStatusPendingPayment &&
			(payment.Status == constants.PaymentStatusPending || payment.Status == constants.PaymentStatusInitiated) &&
			payment.SupersededAt == nil && (payment.ExpiredAt == nil || payment.ExpiredAt.After(now)) &&
			(pending == nil || payment.ID > pending.ID) {
			pending = payment
		}
		// Canceled orders retain the last usable payment quote. Cancellation expires
		// pending links, but must not erase their customer fee and invoice tax.
		if order.PaidAt == nil && order.Status == constants.OrderStatusCanceled &&
			payment.SupersededAt == nil &&
			(payment.Status == constants.PaymentStatusPending || payment.Status == constants.PaymentStatusInitiated || payment.Status == constants.PaymentStatusExpired) &&
			(payment.PayURL != "" || payment.QRCode != "") &&
			(canceled == nil || payment.ID > canceled.ID) {
			canceled = payment
		}
	}
	if pending == nil && !hasSuccessfulPayment {
		pending = canceled
	}
	if order.PaidAt == nil && pending != nil {
		fee, tax = decimal.Zero, decimal.Zero
		if pending.FeePolicy == constants.PaymentFeePolicyCustomerSurcharge || pending.FeePolicy == constants.PaymentFeePolicyLegacyCustomerSurcharge || pending.FeePolicy == "" {
			fee, tax = pending.FeeAmount.Decimal, pending.TaxAmount.Decimal
		}
	}
	order.CustomerFeeAmount, order.PaymentTaxAmount = money.FromDecimal(fee), money.FromDecimal(tax)

	children := make([]*orderdomain.Order, 0, len(order.Children))
	for i := range order.Children {
		children = append(children, &order.Children[i])
	}
	sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })
	weightTotal := decimal.Zero
	for _, child := range children {
		weightTotal = weightTotal.Add(child.TotalAmount.Decimal)
	}
	if !weightTotal.IsPositive() {
		return
	}
	// Cumulative allocation gives the final child any rounding remainder.
	base, allocatedFee, allocatedTax := decimal.Zero, decimal.Zero, decimal.Zero
	for _, child := range children {
		base = base.Add(child.TotalAmount.Decimal)
		nextFee := fee.Mul(base).Div(weightTotal).Round(2)
		nextTax := tax.Mul(base).Div(weightTotal).Round(2)
		child.CustomerFeeAmount = money.FromDecimal(nextFee.Sub(allocatedFee))
		child.PaymentTaxAmount = money.FromDecimal(nextTax.Sub(allocatedTax))
		allocatedFee, allocatedTax = nextFee, nextTax
	}
}
