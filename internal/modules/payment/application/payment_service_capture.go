package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	coupondomain "github.com/dujiao-next/internal/modules/coupon/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"

	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/logger"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

// CapturePaymentInput 捕获支付输入。
type CapturePaymentInput struct {
	PaymentID uint
	Context   context.Context
}

func (s *PaymentService) CapturePayment(input CapturePaymentInput) (*paymentdomain.Payment, error) {
	if input.PaymentID == 0 {
		return nil, ErrPaymentInvalid
	}
	payment, err := s.paymentRepo.GetByID(input.PaymentID)
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if payment == nil {
		return nil, ErrPaymentNotFound
	}
	if payment.Status == constants.PaymentStatusSuccess {
		return payment, nil
	}

	channel, err := s.channelRepo.GetByID(payment.ChannelID)
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if channel == nil {
		return nil, ErrPaymentChannelNotFound
	}

	providerType := strings.ToLower(strings.TrimSpace(channel.ProviderType))
	if providerType != constants.PaymentProviderOfficial {
		return nil, ErrPaymentProviderNotSupported
	}
	if strings.TrimSpace(payment.ProviderRef) == "" {
		return nil, ErrPaymentInvalid
	}

	// 统一通过 Registry 路由。Registry.Lookup 会返回 channel 对应的 adapter,
	// 如果 adapter 不实现 Capturer,type assertion 失败,返回 ErrPaymentProviderNotSupported。
	// 因此无需在此显式检查 channel 是否支持 capture。
	return s.captureViaRegistry(input, payment, channel)
}

// captureViaRegistry 通过 PaymentProviderRegistry 路由调用 QueryPayment。
// stripe + paypal + wechat 实现了 paymentcontract.GatewayCapturer 接口,其它 channel
// (alipay / epay / epusdt / bepusdt / tokenpay / okpay) 仅实现 webhook 回调,
// type assertion 失败时返回 ErrPaymentProviderNotSupported。
func (s *PaymentService) captureViaRegistry(input CapturePaymentInput, payment *paymentdomain.Payment, channel *paymentdomain.PaymentChannel) (*paymentdomain.Payment, error) {
	logger.Infow("payment_capture_via_registry",
		"payment_id", payment.ID,
		"provider_type", channel.ProviderType,
		"channel_type", channel.ChannelType,
	)
	if s.paymentProviderRegistry == nil {
		return nil, ErrPaymentProviderNotSupported
	}
	p, ok := s.paymentProviderRegistry.Lookup(channel.ProviderType, channel.ChannelType)
	if !ok {
		return nil, ErrPaymentProviderNotSupported
	}
	capturer, ok := p.(paymentcontract.GatewayCapturer)
	if !ok {
		logger.Warnw("payment_provider_capture_not_implemented",
			"provider_type", channel.ProviderType,
			"channel_type", channel.ChannelType,
		)
		return nil, ErrPaymentProviderNotSupported
	}

	// 第二参数是 interactionMode,不是 channelType。stripe/wechat adapter
	// 会拒绝任何非法 mode,传 channelType 会导致  一律 ErrConfigInvalid。
	if err := capturer.ValidateConfig(channel.ConfigJSON, channel.InteractionMode); err != nil {
		return nil, mapProviderErrorToService(err)
	}

	ctx, cancel := detachOutboundRequestContext(input.Context)
	defer cancel()

	queryResult, err := capturer.QueryPayment(ctx, channel.ConfigJSON, payment.ProviderRef)
	if err != nil {
		return nil, mapProviderErrorToService(err)
	}

	payload := jsonmap.JSON{}
	if queryResult.Payload != nil {
		payload = queryResult.Payload
	}
	status := strings.TrimSpace(queryResult.Status)
	if status == "" {
		status = constants.PaymentStatusPending
	}

	callbackInput := PaymentCallbackInput{
		PaymentID:   payment.ID,
		ChannelID:   channel.ID,
		Status:      status,
		ProviderRef: pickFirstNonEmpty(queryResult.ProviderRef, payment.ProviderRef),
		Amount:      queryResult.Amount,
		Currency:    strings.ToUpper(strings.TrimSpace(queryResult.Currency)),
		PaidAt:      queryResult.PaidAt,
		Payload:     payload,
	}
	return s.HandleCallback(callbackInput)
}

// mapProviderErrorToService 把 provider.ErrXxx 转换为 service 层的 ErrPaymentXxx。
func mapProviderErrorToService(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, paymentcontract.ErrGatewayConfigInvalid):
		return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
	case errors.Is(err, paymentcontract.ErrGatewayRequestFailed), errors.Is(err, paymentcontract.ErrGatewayAuthFailed):
		return fmt.Errorf("%w: %v", ErrPaymentGatewayRequestFailed, err)
	case errors.Is(err, paymentcontract.ErrGatewayResponseInvalid), errors.Is(err, paymentcontract.ErrGatewaySignatureInvalid):
		return fmt.Errorf("%w: %v", ErrPaymentGatewayResponseInvalid, err)
	case errors.Is(err, paymentcontract.ErrGatewayUnsupportedChannel), errors.Is(err, paymentcontract.ErrGatewayProviderNotFound):
		return ErrPaymentProviderNotSupported
	default:
		return fmt.Errorf("%w: %v", ErrPaymentGatewayRequestFailed, err)
	}
}

var ErrPaymentRecoveryInvalid = errors.New("payment cannot recover closed order")

// RecoverClosedOrder applies a verified late payment to its original canceled order.
// The payment and order locks serialize retries with callbacks and other recoveries.
func (s *PaymentService) RecoverClosedOrder(paymentID, adminID uint) (*paymentdomain.Payment, error) {
	if adminID == 0 {
		return nil, ErrPaymentRecoveryInvalid
	}
	var payment *paymentdomain.Payment
	var order *orderdomain.Order
	recovered := false
	now := time.Now()
	err := s.paymentRepo.WithinTransaction(func(tx paymentcontract.Transaction) error {
		var err error
		payment, err = tx.Payments().GetByIDForUpdate(paymentID)
		if err != nil {
			return err
		}
		if payment == nil || payment.OrderID == 0 || payment.Status != constants.PaymentStatusSuccess {
			return ErrPaymentRecoveryInvalid
		}
		order, err = tx.Orders().GetByIDForUpdateWithChildren(payment.OrderID)
		if err != nil {
			return err
		}
		if order == nil {
			return ErrPaymentRecoveryInvalid
		}
		if payment.ProviderPayload["closed_order_recovered_at"] != nil {
			return nil
		}
		if payment.ExceptionCode != constants.PaymentExceptionClosedOrderSucceeded || order.ParentID != nil || order.Status != constants.OrderStatusCanceled || order.PaidAt != nil || !order.RefundedAmount.Decimal.IsZero() || !order.WalletPaidAmount.Decimal.IsZero() {
			return ErrPaymentRecoveryInvalid
		}
		// Released wallet funds cannot be counted a second time. Only the verified online
		// payment may fund recovery; gateway surcharges are excluded by the shared resolver.
		if paymentCoveredOrderAmount(payment, payment.Amount.Decimal).LessThan(order.TotalAmount.Decimal) {
			return ErrPaymentRecoveryInvalid
		}
		if err := s.reserveRecoveryOrder(tx, order, now); err != nil {
			return err
		}
		if err := restoreRecoveryCoupon(tx, order, now); err != nil {
			return err
		}
		// markOrderPaid owns stock consumption, child status and activation rules.
		order.Status = constants.OrderStatusPendingPayment
		if err := s.markOrderPaid(tx, order, now); err != nil {
			return err
		}
		if s.resellerAccounting != nil {
			if err := s.resellerAccounting.PostOrderProfit(tx.ResellerAccounting(), order, payment); err != nil {
				return err
			}
		}
		recoveredOrders := []*orderdomain.Order{order}
		for i := range order.Children {
			recoveredOrders = append(recoveredOrders, &order.Children[i])
		}
		for _, recoveredOrder := range recoveredOrders {
			if err := tx.Orders().UpdateStatus(recoveredOrder.ID, recoveredOrder.Status, map[string]interface{}{"canceled_at": nil, "expires_at": nil}); err != nil {
				return err
			}
			recoveredOrder.CanceledAt = nil
			recoveredOrder.ExpiresAt = nil
		}
		if payment.ProviderPayload == nil {
			payment.ProviderPayload = jsonmap.JSON{}
		}
		payment.ProviderPayload["closed_order_recovered_at"] = now.Format(time.RFC3339Nano)
		payment.ProviderPayload["closed_order_recovered_by"] = adminID
		payment.ExceptionCode = ""
		if err := tx.Payments().Update(payment); err != nil {
			return err
		}
		recovered = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if recovered {
		log := paymentLogger("payment_id", paymentID, "order_id", order.ID, "admin_id", adminID)
		log.Infow("payment_closed_order_recovered")
		s.enqueueOrderPaidAsync(order, payment, log)
	}
	return payment, nil
}

func (s *PaymentService) reserveRecoveryOrder(tx paymentcontract.Transaction, order *orderdomain.Order, now time.Time) error {
	if order.Status != constants.OrderStatusCanceled || order.PaidAt != nil || !order.RefundedAmount.Decimal.IsZero() {
		return ErrPaymentRecoveryInvalid
	}
	fulfillment, err := tx.Fulfillments().GetByOrderID(order.ID)
	if err != nil {
		return err
	}
	if fulfillment != nil {
		return ErrPaymentRecoveryInvalid
	}
	if len(order.Children) > 0 {
		for i := range order.Children {
			if err := s.reserveRecoveryOrder(tx, &order.Children[i], now); err != nil {
				return err
			}
		}
		return nil
	}
	if len(order.Items) == 0 {
		return ErrPaymentRecoveryInvalid
	}
	for _, item := range order.Items {
		if item.Quantity <= 0 {
			return ErrPaymentRecoveryInvalid
		}
		product, err := tx.Products().GetByID(strconv.FormatUint(uint64(item.ProductID), 10))
		if err != nil {
			return err
		}
		if product == nil || !product.IsActive {
			return ErrPaymentRecoveryInvalid
		}
		manualTotal := product.ManualStockTotal
		if item.SKUID > 0 {
			sku, err := tx.ProductSKUs().GetByID(item.SKUID)
			if err != nil {
				return err
			}
			if sku == nil || !sku.IsActive || sku.ProductID != item.ProductID {
				return ErrPaymentRecoveryInvalid
			}
			manualTotal = sku.ManualStockTotal
		}
		switch item.FulfillmentType {
		case constants.FulfillmentTypeUpstream:
			if s.procurementSvc == nil || s.queue == nil || !s.queue.Enabled() {
				return ErrPaymentRecoveryInvalid
			}
		case constants.FulfillmentTypeAuto:
			if s.queue == nil || !s.queue.Enabled() {
				return ErrPaymentRecoveryInvalid
			}
			rows, err := tx.CardSecrets().ListAvailableByProductForUpdate(item.ProductID, item.SKUID, item.Quantity)
			if err != nil {
				return err
			}
			if len(rows) != item.Quantity {
				return ErrPaymentRecoveryInvalid
			}
			ids := make([]uint, len(rows))
			for i := range rows {
				ids[i] = rows[i].ID
			}
			affected, err := tx.CardSecrets().Reserve(ids, order.ID, now)
			if err != nil {
				return err
			}
			if affected != int64(item.Quantity) {
				return ErrPaymentRecoveryInvalid
			}
		case constants.FulfillmentTypeManual:
			if manualTotal == constants.ManualStockUnlimited {
				continue
			}
			var affected int64
			if item.SKUID > 0 {
				affected, err = tx.ProductSKUs().ReserveManualStock(item.SKUID, item.Quantity)
			} else {
				affected, err = tx.Products().ReserveManualStock(item.ProductID, item.Quantity)
			}
			if err != nil {
				return err
			}
			if affected != 1 {
				return ErrPaymentRecoveryInvalid
			}
		default:
			return ErrPaymentRecoveryInvalid
		}
	}
	return nil
}

func restoreRecoveryCoupon(tx paymentcontract.Transaction, order *orderdomain.Order, now time.Time) error {
	if order.CouponID == nil {
		return nil
	}
	coupon, err := tx.Coupons().GetByIDForUpdate(*order.CouponID)
	if err != nil {
		return err
	}
	if coupon == nil || (coupon.UsageLimit > 0 && coupon.UsedCount >= coupon.UsageLimit) {
		return ErrPaymentRecoveryInvalid
	}
	usages, err := tx.CouponUsages().ListByOrderID(order.ID)
	if err != nil {
		return err
	}
	if len(usages) > 0 {
		return ErrPaymentRecoveryInvalid
	}
	if coupon.PerUserLimit > 0 && order.UserID != 0 {
		count, err := tx.CouponUsages().CountByUser(coupon.ID, order.UserID)
		if err != nil {
			return err
		}
		if count >= int64(coupon.PerUserLimit) {
			return ErrPaymentRecoveryInvalid
		}
	}
	if err := tx.CouponUsages().Create(&coupondomain.CouponUsage{CouponID: coupon.ID, UserID: order.UserID, OrderID: order.ID, DiscountAmount: order.DiscountAmount, CreatedAt: now}); err != nil {
		return err
	}
	return tx.Coupons().IncrementUsedCount(coupon.ID, 1)
}
