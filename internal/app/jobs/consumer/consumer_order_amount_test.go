package consumer

import (
	"testing"

	"github.com/dujiao-next/internal/constants"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
)

func emailAmount(value string) money.Amount {
	return money.FromDecimal(decimal.RequireFromString(value))
}

func TestOrderStatusEmailAmountUsesActualPaid(t *testing.T) {
	order := &orderdomain.Order{ID: 1, TotalAmount: emailAmount("10.00")}
	payments := []paymentdomain.Payment{
		{OrderID: 1, Status: constants.PaymentStatusSuccess, Amount: emailAmount("10.20")},
	}
	if got := orderStatusEmailAmount(order, payments); got.String() != "10.20" {
		t.Fatalf("online paid amount = %s, want 10.20", got.String())
	}

	// 钱包 + 在线混合支付：两者相加才是用户实际支付金额。
	order.WalletPaidAmount = emailAmount("3.00")
	payments[0].Amount = emailAmount("7.20")
	if got := orderStatusEmailAmount(order, payments); got.String() != "10.20" {
		t.Fatalf("mixed paid amount = %s, want 10.20", got.String())
	}

	// 未支付（没有成功支付单、钱包也没扣）时回退订单金额。
	empty := &orderdomain.Order{ID: 2, TotalAmount: emailAmount("10.00")}
	if got := orderStatusEmailAmount(empty, nil); got.String() != "10.00" {
		t.Fatalf("unpaid fallback = %s, want 10.00", got.String())
	}
}

func TestOrderStatusEmailAmountSkipsExceptionAndForeignPayments(t *testing.T) {
	order := &orderdomain.Order{ID: 1, TotalAmount: emailAmount("10.00")}
	payments := []paymentdomain.Payment{
		{OrderID: 1, Status: constants.PaymentStatusSuccess, Amount: emailAmount("10.20"), ExceptionCode: constants.PaymentExceptionUnderpaidSucceeded},
		{OrderID: 2, Status: constants.PaymentStatusSuccess, Amount: emailAmount("99.00")},
	}
	if got := orderStatusEmailAmount(order, payments); got.String() != "10.00" {
		t.Fatalf("exception/foreign payments must not count, got %s", got.String())
	}
}

func TestResolveOrderStatusEmailAmountOnlyUsesPaidStatuses(t *testing.T) {
	order := &orderdomain.Order{ID: 1, TotalAmount: emailAmount("10.00")}
	c := &Consumer{}
	// 非已支付状态直接使用订单金额，不查询支付记录。
	for _, status := range []string{constants.OrderStatusRefunded, constants.OrderStatusPartiallyRefunded} {
		if got := c.resolveOrderStatusEmailAmount(order, status); got.String() != "10.00" {
			t.Fatalf("%s amount = %s, want 10.00", status, got.String())
		}
	}
}
