package contract

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

var ErrExchangeRateInvalid = errors.New("exchange rate configuration invalid")
var ErrExchangeRateUnavailable = errors.New("exchange rate unavailable")

const ExchangeRateConfigKey = "exchange_rate_config"
const ExchangeRateStateKey = "exchange_rate_state"

type ExchangeRateSource struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}

type ExchangeRateConfig struct {
	Enabled             bool                 `json:"enabled"`
	QuoteCurrency       string               `json:"quote_currency"`
	MarkupPercent       decimal.Decimal      `json:"markup_percent"`
	MarkupAmount        decimal.Decimal      `json:"markup_amount"`
	RefreshMinutes      int                  `json:"refresh_minutes"`
	MaxDeviationPercent decimal.Decimal      `json:"max_deviation_percent"`
	MaxChangePercent    decimal.Decimal      `json:"max_change_percent"`
	ManualRate          decimal.Decimal      `json:"manual_rate"`
	FallbackRate        decimal.Decimal      `json:"fallback_rate"`
	Sources             []ExchangeRateSource `json:"sources"`
}

type ExchangeRateQuote struct {
	ID          string          `json:"id"`
	Rate        decimal.Decimal `json:"rate"`
	PublishedAt time.Time       `json:"published_at"`
	Status      string          `json:"status"`
}

type ExchangeRateSnapshot struct {
	MarkupPercent decimal.Decimal     `json:"markup_percent"`
	MarketRate    decimal.Decimal     `json:"market_rate"`
	Rate          decimal.Decimal     `json:"rate"`
	MarkupAmount  decimal.Decimal     `json:"markup_amount"`
	UpdatedAt     time.Time           `json:"updated_at"`
	RefreshedAt   time.Time           `json:"refreshed_at"`
	Mode          string              `json:"mode"`
	Sources       []ExchangeRateQuote `json:"sources"`
}

type ExchangeRateFetcher interface {
	Fetch(context.Context, ExchangeRateSource, string) (ExchangeRateQuote, error)
}

type ExchangeRateProvider interface {
	GetExchangeRate(context.Context, bool) (ExchangeRateSnapshot, error)
}
