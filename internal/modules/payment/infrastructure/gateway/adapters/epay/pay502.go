package epayadapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/dujiao-next/internal/constants"
	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

// pay502Adapter implements the 502 Epay V1 API and notification protocol.
type pay502Adapter struct{ epayAdapter }

func NewPay502Adapter() paymentcontract.GatewayProvider { return &pay502Adapter{} }

func (a *pay502Adapter) Type() string { return constants.PaymentProvider502 + ":" }

func pay502Config(raw jsonmap.JSON) (jsonmap.JSON, error) {
	cfg := jsonmap.JSON{}
	for key, value := range raw {
		cfg[key] = value
	}
	for key, expected := range map[string]string{"epay_version": "v1", "sign_type": "MD5"} {
		if value, exists := cfg[key]; exists && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" && !strings.EqualFold(strings.TrimSpace(fmt.Sprint(value)), expected) {
			return nil, fmt.Errorf("%w: pay502 %s must be %s", paymentcontract.ErrGatewayConfigInvalid, key, expected)
		}
		cfg[key] = expected
	}
	if strings.TrimSpace(fmt.Sprint(cfg["gateway_url"])) == "" || cfg["gateway_url"] == nil {
		cfg["gateway_url"] = "https://pay.502k.cn/xpay/epay"
	}
	if currency, _ := cfg["target_currency"].(string); strings.TrimSpace(currency) != "" && !strings.EqualFold(strings.TrimSpace(currency), "CNY") {
		return nil, fmt.Errorf("%w: pay502 requires CNY", paymentcontract.ErrGatewayConfigInvalid)
	}
	return cfg, nil
}

func (a *pay502Adapter) ValidateConfig(raw jsonmap.JSON, channelType string) error {
	cfg, err := pay502Config(raw)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(channelType)) {
	case "", constants.PaymentChannelTypeAlipay, constants.PaymentChannelTypeWechat, constants.PaymentChannelTypeWxpay:
	default:
		return fmt.Errorf("%w: 502 channel_type %s", paymentcontract.ErrGatewayUnsupportedChannel, channelType)
	}
	return a.epayAdapter.ValidateConfig(cfg, channelType)
}

func (a *pay502Adapter) CreatePayment(ctx context.Context, raw jsonmap.JSON, input paymentcontract.GatewayCreateInput) (*paymentcontract.GatewayCreateResult, error) {
	cfg, err := pay502Config(raw)
	if err != nil {
		return nil, err
	}
	if err := a.ValidateConfig(cfg, input.ChannelType); err != nil {
		return nil, err
	}
	parsed, err := a.parseConfig(cfg)
	if err != nil {
		return nil, err
	}
	// Reject unsupported currencies before issuing an external payment request.
	if !strings.EqualFold(strings.TrimSpace(input.Currency), "CNY") && !parsed.NeedsCurrencyConversion() {
		return nil, fmt.Errorf("%w: 502 requires CNY or currency conversion to CNY", paymentcontract.ErrGatewayConfigInvalid)
	}
	result, err := a.epayAdapter.CreatePayment(ctx, cfg, input)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(result.CurrencySent), "CNY") {
		return nil, fmt.Errorf("%w: pay502 requires CNY or currency conversion to CNY", paymentcontract.ErrGatewayConfigInvalid)
	}
	return result, nil
}

func (a *pay502Adapter) VerifyCallback(raw jsonmap.JSON, form map[string][]string, body []byte) (*paymentcontract.GatewayCallbackResult, error) {
	cfg, err := pay502Config(raw)
	if err != nil {
		return nil, err
	}
	result, err := a.epayAdapter.VerifyCallback(cfg, form, body)
	if err != nil {
		return nil, err
	}
	// Only TRADE_SUCCESS is documented as a successful 502 payment.
	if values := form["trade_status"]; len(values) == 0 || values[0] != "TRADE_SUCCESS" {
		result.Status = constants.PaymentStatusPending
	}
	return result, nil
}
