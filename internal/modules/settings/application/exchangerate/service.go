package exchangerate

import (
	"context"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	settingscontract "github.com/dujiao-next/internal/modules/settings/contract"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/shopspring/decimal"
)

// ExchangeRateService serializes refreshes and persists the last accepted market rate.
// All reads and HTTP requests finish before callers enter financial transactions.
type Settings interface {
	GetByKey(string) (jsonmap.JSON, error)
	Update(string, map[string]interface{}) (jsonmap.JSON, error)
}

type ExchangeRateService struct {
	settings Settings
	fetcher  settingscontract.ExchangeRateFetcher
	mu       sync.Mutex
}

func NewExchangeRateService(settings Settings, fetcher settingscontract.ExchangeRateFetcher) *ExchangeRateService {
	return &ExchangeRateService{settings: settings, fetcher: fetcher}
}

func DefaultExchangeRateConfig() settingscontract.ExchangeRateConfig {
	return settingscontract.ExchangeRateConfig{
		Enabled: true, QuoteCurrency: "CNY", MarkupAmount: decimal.RequireFromString("0.2"), RefreshMinutes: 30,
		MaxDeviationPercent: decimal.NewFromInt(1), MaxChangePercent: decimal.NewFromInt(1),
		Sources: []settingscontract.ExchangeRateSource{
			{ID: "frankfurter", Enabled: true, URL: "https://api.frankfurter.dev/v1/latest?base=USD&symbols=CNY"},
			{ID: "open_er", Enabled: true, URL: "https://open.er-api.com/v6/latest/USD"},
			{ID: "currency_api", Enabled: true, URL: "https://cdn.jsdelivr.net/npm/@fawazahmed0/currency-api@latest/v1/currencies/usd.json"},
			{ID: "exchangerate_fun", Enabled: true, URL: "https://api.exchangerate.fun/latest"},
		},
	}
}

func (s *ExchangeRateService) GetConfig() (settingscontract.ExchangeRateConfig, error) {
	cfg := DefaultExchangeRateConfig()
	value, err := s.settings.GetByKey(settingscontract.ExchangeRateConfigKey)
	if err != nil {
		return cfg, err
	}
	if value != nil {
		if err = decodeExchangeJSON(value, &cfg); err != nil {
			return cfg, err
		}
	}
	return cfg, validateExchangeRateConfig(cfg)
}

func validateExchangeRateConfig(cfg settingscontract.ExchangeRateConfig) error {
	if cfg.QuoteCurrency != "CNY" || cfg.RefreshMinutes < 1 || cfg.RefreshMinutes > 1440 ||
		cfg.MarkupPercent.IsNegative() || cfg.MarkupPercent.GreaterThan(decimal.NewFromInt(100)) ||
		cfg.MarkupAmount.IsNegative() || cfg.MarkupAmount.GreaterThan(decimal.NewFromInt(100)) ||
		cfg.MaxDeviationPercent.IsNegative() || cfg.MaxDeviationPercent.GreaterThan(decimal.NewFromInt(100)) ||
		cfg.MaxChangePercent.IsNegative() || cfg.MaxChangePercent.GreaterThan(decimal.NewFromInt(100)) ||
		cfg.ManualRate.IsNegative() || cfg.FallbackRate.IsNegative() || len(cfg.Sources) > 4 {
		return settingscontract.ErrExchangeRateInvalid
	}
	seen := map[string]bool{}
	for _, source := range cfg.Sources {
		if seen[source.ID] {
			return settingscontract.ErrExchangeRateInvalid
		}
		seen[source.ID] = true
		switch source.ID {
		case "frankfurter", "open_er", "currency_api", "exchangerate_fun":
		default:
			return settingscontract.ErrExchangeRateInvalid
		}
		u, err := url.Parse(source.URL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || len(source.URL) > 2048 {
			return settingscontract.ErrExchangeRateInvalid
		}
	}
	return nil
}

func (s *ExchangeRateService) UpdateConfig(cfg settingscontract.ExchangeRateConfig) (settingscontract.ExchangeRateConfig, error) {
	cfg.QuoteCurrency = strings.ToUpper(strings.TrimSpace(cfg.QuoteCurrency))
	if err := validateExchangeRateConfig(cfg); err != nil {
		return cfg, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, err := encodeExchangeJSON(cfg)
	if err != nil {
		return cfg, err
	}
	_, err = s.settings.Update(settingscontract.ExchangeRateConfigKey, value)
	if err != nil {
		return cfg, err
	}
	raw, err := s.settings.GetByKey(settingscontract.ExchangeRateStateKey)
	if err != nil {
		return cfg, err
	}
	if raw != nil {
		state := settingscontract.ExchangeRateSnapshot{}
		if err = decodeExchangeJSON(raw, &state); err != nil {
			return cfg, err
		}
		state.RefreshedAt = time.Time{}
		value, err = encodeExchangeJSON(state)
		if err != nil {
			return cfg, err
		}
		_, err = s.settings.Update(settingscontract.ExchangeRateStateKey, value)
	}
	return cfg, err
}

func (s *ExchangeRateService) GetExchangeRate(ctx context.Context, force bool) (settingscontract.ExchangeRateSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.GetConfig()
	if err != nil {
		return settingscontract.ExchangeRateSnapshot{}, err
	}
	if !cfg.Enabled {
		return settingscontract.ExchangeRateSnapshot{}, settingscontract.ErrExchangeRateUnavailable
	}
	now := time.Now().UTC()
	previous := settingscontract.ExchangeRateSnapshot{}
	raw, err := s.settings.GetByKey(settingscontract.ExchangeRateStateKey)
	if err != nil {
		return previous, err
	}
	if raw != nil {
		if err = decodeExchangeJSON(raw, &previous); err != nil {
			return previous, err
		}
	}
	snapshot := previous
	snapshot.MarkupAmount = cfg.MarkupAmount
	snapshot.MarkupPercent = cfg.MarkupPercent
	if cfg.ManualRate.IsPositive() {
		snapshot.MarketRate = cfg.ManualRate
		snapshot.UpdatedAt = now
		snapshot.Mode = "manual"
	} else if !force && now.Sub(previous.RefreshedAt) < time.Duration(cfg.RefreshMinutes)*time.Minute && previous.Mode != "manual" {
		// Reapply current markup even if the cached market quote is still fresh.
	} else {
		snapshot = s.refresh(ctx, cfg, previous, now)
	}
	snapshot.Rate = snapshot.MarketRate.Mul(decimal.NewFromInt(1).Add(cfg.MarkupPercent.Div(decimal.NewFromInt(100)))).Add(cfg.MarkupAmount).Round(6)
	snapshot.MarkupAmount = cfg.MarkupAmount
	snapshot.MarkupPercent = cfg.MarkupPercent
	value, err := encodeExchangeJSON(snapshot)
	if err != nil {
		return snapshot, err
	}
	if _, err = s.settings.Update(settingscontract.ExchangeRateStateKey, value); err != nil {
		return snapshot, err
	}
	if !snapshot.MarketRate.IsPositive() || !snapshot.Rate.IsPositive() {
		return snapshot, settingscontract.ErrExchangeRateUnavailable
	}
	return snapshot, nil
}

func (s *ExchangeRateService) refresh(ctx context.Context, cfg settingscontract.ExchangeRateConfig, previous settingscontract.ExchangeRateSnapshot, now time.Time) settingscontract.ExchangeRateSnapshot {
	snapshot := settingscontract.ExchangeRateSnapshot{RefreshedAt: now, Sources: []settingscontract.ExchangeRateQuote{}}
	var rates []decimal.Decimal
	for _, source := range cfg.Sources {
		if !source.Enabled {
			continue
		}
		quote, err := s.fetcher.Fetch(ctx, source, cfg.QuoteCurrency)
		quote.ID = source.ID
		quote.Status = "ok"
		if err != nil || !quote.Rate.IsPositive() {
			quote.Status = "error"
		} else if quote.PublishedAt.IsZero() || now.Sub(quote.PublishedAt) > 7*24*time.Hour || quote.PublishedAt.After(now.Add(5*time.Minute)) {
			quote.Status = "stale"
		} else {
			rates = append(rates, quote.Rate)
		}
		snapshot.Sources = append(snapshot.Sources, quote)
	}
	if len(rates) > 0 {
		sort.Slice(rates, func(i, j int) bool { return rates[i].LessThan(rates[j]) })
		median := rates[len(rates)/2]
		if len(rates)%2 == 0 {
			median = median.Add(rates[len(rates)/2-1]).Div(decimal.NewFromInt(2))
		}
		total := decimal.Zero
		count := int64(0)
		for i := range snapshot.Sources {
			q := &snapshot.Sources[i]
			if q.Status != "ok" {
				continue
			}
			if cfg.MaxDeviationPercent.IsPositive() && percentDifference(q.Rate, median).GreaterThan(cfg.MaxDeviationPercent) {
				q.Status = "outlier"
				continue
			}
			total = total.Add(q.Rate)
			count++
		}
		if count > 0 {
			candidate := total.Div(decimal.NewFromInt(count)).Round(8)
			if (previous.Mode == "aggregate" || previous.Mode == "cached") && previous.MarketRate.IsPositive() && cfg.MaxChangePercent.IsPositive() && percentDifference(candidate, previous.MarketRate).GreaterThan(cfg.MaxChangePercent) {
				for i := range snapshot.Sources {
					if snapshot.Sources[i].Status == "ok" {
						snapshot.Sources[i].Status = "change_rejected"
					}
				}
			} else {
				snapshot.MarketRate = candidate
				snapshot.UpdatedAt = now
				snapshot.Mode = "aggregate"
				return snapshot
			}
		}
	}
	if previous.MarketRate.IsPositive() && (previous.Mode == "aggregate" || previous.Mode == "cached") && now.Sub(previous.UpdatedAt) < 24*time.Hour {
		snapshot.MarketRate = previous.MarketRate
		snapshot.UpdatedAt = previous.UpdatedAt
		snapshot.Mode = "cached"
	} else if cfg.FallbackRate.IsPositive() {
		snapshot.MarketRate = cfg.FallbackRate
		snapshot.UpdatedAt = now
		snapshot.Mode = "fallback"
	}
	return snapshot
}

func percentDifference(value, base decimal.Decimal) decimal.Decimal {
	return value.Sub(base).Abs().Div(base).Mul(decimal.NewFromInt(100))
}
func encodeExchangeJSON(value any) (jsonmap.JSON, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out jsonmap.JSON
	err = json.Unmarshal(data, &out)
	return out, err
}
func decodeExchangeJSON(value jsonmap.JSON, target any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
