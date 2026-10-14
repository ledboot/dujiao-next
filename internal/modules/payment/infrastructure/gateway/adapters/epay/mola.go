package epayadapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/dujiao-next/internal/constants"
	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

// molaAdapter uses Mola's documented Epay V1 redirect and notification protocol.
// Mola does not document the mapi.php QR endpoint or the V2 RSA protocol.
type molaAdapter struct{ epayAdapter }

func NewMolaAdapter() paymentcontract.GatewayProvider { return &molaAdapter{} }

func (a *molaAdapter) Type() string { return constants.PaymentProviderMola + ":" }

func molaConfig(raw jsonmap.JSON) (jsonmap.JSON, error) {
	cfg := jsonmap.JSON{}
	for key, value := range raw {
		cfg[key] = value
	}
	for key, expected := range map[string]string{"epay_version": "v1", "sign_type": "MD5"} {
		if value, exists := cfg[key]; exists && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" && !strings.EqualFold(strings.TrimSpace(fmt.Sprint(value)), expected) {
			return nil, fmt.Errorf("%w: mola %s must be %s", paymentcontract.ErrGatewayConfigInvalid, key, expected)
		}
		cfg[key] = expected
	}
	if strings.TrimSpace(fmt.Sprint(cfg["gateway_url"])) == "" || cfg["gateway_url"] == nil {
		cfg["gateway_url"] = "https://pay.mygptlife.com"
	}
	if currency, _ := cfg["target_currency"].(string); strings.TrimSpace(currency) != "" && !strings.EqualFold(strings.TrimSpace(currency), "CNY") {
		return nil, fmt.Errorf("%w: mola requires CNY", paymentcontract.ErrGatewayConfigInvalid)
	}
	return cfg, nil
}

func (a *molaAdapter) ValidateConfig(raw jsonmap.JSON, channelType string) error {
	cfg, err := molaConfig(raw)
	if err != nil {
		return err
	}
	return a.epayAdapter.ValidateConfig(cfg, channelType)
}

func (a *molaAdapter) CreatePayment(ctx context.Context, raw jsonmap.JSON, input paymentcontract.GatewayCreateInput) (*paymentcontract.GatewayCreateResult, error) {
	cfg, err := molaConfig(raw)
	if err != nil {
		return nil, err
	}
	mode, _ := input.Extra["interaction_mode"].(string)
	if !strings.EqualFold(strings.TrimSpace(mode), constants.PaymentInteractionRedirect) {
		return nil, fmt.Errorf("%w: mola requires redirect", paymentcontract.ErrGatewayConfigInvalid)
	}
	result, err := a.epayAdapter.CreatePayment(ctx, cfg, input)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(result.CurrencySent), "CNY") {
		return nil, fmt.Errorf("%w: mola requires CNY or currency conversion to CNY", paymentcontract.ErrGatewayConfigInvalid)
	}
	return result, nil
}

func (a *molaAdapter) VerifyCallback(raw jsonmap.JSON, form map[string][]string, body []byte) (*paymentcontract.GatewayCallbackResult, error) {
	cfg, err := molaConfig(raw)
	if err != nil {
		return nil, err
	}
	result, err := a.epayAdapter.VerifyCallback(cfg, form, body)
	if err != nil {
		return nil, err
	}
	// Only TRADE_SUCCESS is documented as a successful Mola payment.
	if values := form["trade_status"]; len(values) == 0 || values[0] != "TRADE_SUCCESS" {
		result.Status = constants.PaymentStatusPending
	}
	return result, nil
}
