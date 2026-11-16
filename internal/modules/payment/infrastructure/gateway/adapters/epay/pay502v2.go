package epayadapter

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"
	gatewaycommon "github.com/dujiao-next/internal/modules/payment/infrastructure/gateway/common"
	"github.com/dujiao-next/internal/modules/payment/infrastructure/gateway/epay"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

// pay502v2Adapter implements the 502 Epay V2 API (RSA-SHA256 signing).
// Only two delivery methods are supported: jump (redirect) and scan (QR code).
type pay502v2Adapter struct{ epayAdapter }

const (
	pay502v2DefaultGatewayURL = "https://pay.502k.cn/xpay/epayn"
	pay502v2MethodJump        = "jump"
	pay502v2MethodScan        = "scan"

	// pay502v2CallbackMaxSkew 限制回调 timestamp 与当前时间的偏差，避免重放旧回调。
	pay502v2CallbackMaxSkew = 10 * time.Minute
)

func NewPay502V2Adapter() paymentcontract.GatewayProvider { return &pay502v2Adapter{} }

func (a *pay502v2Adapter) Type() string { return constants.PaymentProvider502V2 + ":" }

func pay502v2Config(raw jsonmap.JSON) (jsonmap.JSON, error) {
	cfg := jsonmap.JSON{}
	for key, value := range raw {
		cfg[key] = value
	}
	for key, expected := range map[string]string{"epay_version": "v2", "sign_type": "RSA"} {
		if value, exists := cfg[key]; exists && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" && !strings.EqualFold(strings.TrimSpace(fmt.Sprint(value)), expected) {
			return nil, fmt.Errorf("%w: pay502v2 %s must be %s", paymentcontract.ErrGatewayConfigInvalid, key, expected)
		}
		cfg[key] = expected
	}
	if cfg["gateway_url"] == nil || strings.TrimSpace(fmt.Sprint(cfg["gateway_url"])) == "" {
		cfg["gateway_url"] = pay502v2DefaultGatewayURL
	}
	if currency, _ := cfg["target_currency"].(string); strings.TrimSpace(currency) != "" && !strings.EqualFold(strings.TrimSpace(currency), "CNY") {
		return nil, fmt.Errorf("%w: pay502v2 requires CNY", paymentcontract.ErrGatewayConfigInvalid)
	}
	return cfg, nil
}

func pay502v2SupportsChannelType(channelType string) bool {
	switch strings.ToLower(strings.TrimSpace(channelType)) {
	case constants.PaymentChannelTypeAlipay, constants.PaymentChannelTypeWechat, constants.PaymentChannelTypeWxpay:
		return true
	default:
		return false
	}
}

func (a *pay502v2Adapter) ValidateConfig(raw jsonmap.JSON, channelType string) error {
	cfg, err := pay502v2Config(raw)
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(channelType)) {
	case "", constants.PaymentChannelTypeAlipay, constants.PaymentChannelTypeWechat, constants.PaymentChannelTypeWxpay:
	default:
		return fmt.Errorf("%w: 502v2 channel_type %s", paymentcontract.ErrGatewayUnsupportedChannel, channelType)
	}
	return a.epayAdapter.ValidateConfig(cfg, channelType)
}

func pay502v2Method(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case constants.PaymentInteractionRedirect:
		return pay502v2MethodJump, nil
	case "", constants.PaymentInteractionQR:
		return pay502v2MethodScan, nil
	default:
		return "", fmt.Errorf("%w: 502v2 interaction_mode %s", paymentcontract.ErrGatewayConfigInvalid, mode)
	}
}

func (a *pay502v2Adapter) CreatePayment(ctx context.Context, raw jsonmap.JSON, input paymentcontract.GatewayCreateInput) (*paymentcontract.GatewayCreateResult, error) {
	cfg, err := pay502v2Config(raw)
	if err != nil {
		return nil, err
	}
	if err := a.ValidateConfig(cfg, input.ChannelType); err != nil {
		return nil, err
	}
	if !pay502v2SupportsChannelType(input.ChannelType) {
		return nil, fmt.Errorf("%w: 502v2 channel_type %s", paymentcontract.ErrGatewayUnsupportedChannel, input.ChannelType)
	}
	parsed, err := a.parseConfig(cfg)
	if err != nil {
		return nil, err
	}
	// Reject unsupported currencies before issuing an external payment request.
	if !strings.EqualFold(strings.TrimSpace(input.Currency), "CNY") && !parsed.NeedsCurrencyConversion() {
		return nil, fmt.Errorf("%w: 502v2 requires CNY or currency conversion to CNY", paymentcontract.ErrGatewayConfigInvalid)
	}

	mode, _ := input.Extra["interaction_mode"].(string)
	method, err := pay502v2Method(mode)
	if err != nil {
		return nil, err
	}
	parsed.Method = method

	notifyURL := strings.TrimSpace(input.NotifyURL)
	if notifyURL == "" {
		notifyURL = strings.TrimSpace(parsed.NotifyURL)
	}
	returnURL := strings.TrimSpace(input.ReturnURL)
	if returnURL == "" {
		returnURL = strings.TrimSpace(parsed.ReturnURL)
	}
	returnURL = gatewaycommon.AppendQueryParams(returnURL, input.ReturnURLQuery)

	originalAmount := input.Amount.Decimal.String()
	originalCurrency := input.Currency
	payAmount := originalAmount
	payCurrency := originalCurrency
	converted := false
	if parsed.NeedsCurrencyConversion() {
		convAmount, convCurrency, convErr := parsed.ConvertAmount(payAmount, payCurrency, 2)
		if convErr != nil {
			return nil, fmt.Errorf("%w: %v", paymentcontract.ErrGatewayConfigInvalid, convErr)
		}
		payAmount = convAmount
		payCurrency = convCurrency
		converted = true
	}

	native := epay.CreateInput{
		OrderNo:     input.OrderNo,
		Amount:      payAmount,
		Subject:     input.Subject,
		ChannelType: input.ChannelType,
		ClientIP:    input.ClientIP,
		NotifyURL:   notifyURL,
		ReturnURL:   returnURL,
	}
	result, err := epay.CreatePayment(ctx, parsed, native)
	if err != nil {
		return nil, mapEpayError(err)
	}
	if !strings.EqualFold(strings.TrimSpace(payCurrency), "CNY") {
		return nil, fmt.Errorf("%w: 502v2 requires CNY or currency conversion to CNY", paymentcontract.ErrGatewayConfigInvalid)
	}
	// The platform signs its response with the platform private key; verify it
	// whenever the gateway returns a signature.
	if err := pay502v2VerifyResponse(result.Raw, parsed.PublicKey); err != nil {
		return nil, err
	}
	// Enforce the requested delivery method instead of silently returning the other one.
	if method == pay502v2MethodScan && strings.TrimSpace(result.QRCode) == "" {
		return nil, fmt.Errorf("%w: 502v2 scan response missing qrcode", paymentcontract.ErrGatewayResponseInvalid)
	}
	if method == pay502v2MethodJump && strings.TrimSpace(result.PayURL) == "" {
		return nil, fmt.Errorf("%w: 502v2 jump response missing pay link", paymentcontract.ErrGatewayResponseInvalid)
	}

	payload := jsonmap.JSON{}
	if result.Raw != nil {
		payload = jsonmap.JSON(result.Raw)
	}
	if converted {
		payload["exchange_rate"] = strings.TrimSpace(parsed.ExchangeRate)
		payload["original_amount"] = originalAmount
		payload["original_currency"] = originalCurrency
	}

	return &paymentcontract.GatewayCreateResult{
		ProviderRef:  result.TradeNo,
		RedirectURL:  result.PayURL,
		QRCodeURL:    result.QRCode,
		Payload:      payload,
		AmountSent:   payAmount,
		CurrencySent: payCurrency,
	}, nil
}

func pay502v2VerifyResponse(raw map[string]interface{}, platformPublicKey string) error {
	if len(raw) == 0 {
		return nil
	}
	params := make(map[string]string, len(raw))
	sign := ""
	for key, value := range raw {
		text := pay502v2ValueText(value)
		if key == "sign" {
			sign = strings.TrimSpace(text)
			continue
		}
		// The platform signs the business fields only: the envelope "code" and
		// sign metadata are excluded (verified against a live V2 response).
		if key == "code" || key == "sign_type" || strings.TrimSpace(text) == "" {
			continue
		}
		params[key] = text
	}
	if sign == "" {
		return nil
	}
	if err := epay.VerifyRSASignature(epay.BuildSignContent(params), sign, platformPublicKey); err != nil {
		return fmt.Errorf("%w: 502v2 response signature invalid", paymentcontract.ErrGatewaySignatureInvalid)
	}
	return nil
}

func pay502v2ValueText(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return fmt.Sprint(v)
	}
}

func (a *pay502v2Adapter) VerifyCallback(raw jsonmap.JSON, form map[string][]string, body []byte) (*paymentcontract.GatewayCallbackResult, error) {
	cfg, err := pay502v2Config(raw)
	if err != nil {
		return nil, err
	}
	result, err := a.epayAdapter.VerifyCallback(cfg, form, body)
	if err != nil {
		return nil, err
	}
	stale, err := pay502v2ValidateCallbackTimestamp(form, time.Now())
	if err != nil {
		return nil, err
	}
	if stale {
		// 502 后台人工补单/平台重试的旧通知仍然有效，只打标记不拒收，
		// 避免超过 10 分钟的真实到账无法自动入账。
		if result.Payload == nil {
			result.Payload = jsonmap.JSON{}
		}
		result.Payload["callback_timestamp_stale"] = true
	}
	// Only TRADE_SUCCESS is documented as a successful 502 payment.
	if values := form["trade_status"]; len(values) == 0 || values[0] != constants.EpayTradeStatusSuccess {
		result.Status = constants.PaymentStatusPending
	}
	return result, nil
}

// pay502v2ValidateCallbackTimestamp 校验回调时间戳。
// 平台回调未携带 timestamp 时跳过（签名已校验）；无法解析或时间在未来超过
// 窗口时拒绝；早于窗口的旧通知返回 stale=true，由调用方标记但不拒收——
// 502 后台人工补单、平台延迟重试都会产生旧通知，不能因此丢掉真实到账。
func pay502v2ValidateCallbackTimestamp(form map[string][]string, now time.Time) (stale bool, err error) {
	raw := strings.TrimSpace(gatewaycommon.FormValue(form, "timestamp"))
	if raw == "" {
		return false, nil
	}
	seconds, parseErr := strconv.ParseInt(raw, 10, 64)
	if parseErr != nil {
		return false, fmt.Errorf("%w: 502v2 callback timestamp invalid", paymentcontract.ErrGatewaySignatureInvalid)
	}
	if seconds > 1_000_000_000_000 { // 兼容毫秒时间戳
		seconds /= 1000
	}
	at := time.Unix(seconds, 0)
	if at.After(now.Add(pay502v2CallbackMaxSkew)) {
		return false, fmt.Errorf("%w: 502v2 callback timestamp in the future", paymentcontract.ErrGatewaySignatureInvalid)
	}
	return at.Before(now.Add(-pay502v2CallbackMaxSkew)), nil
}
