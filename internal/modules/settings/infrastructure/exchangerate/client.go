package exchangerate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	settingscontract "github.com/dujiao-next/internal/modules/settings/contract"
	"github.com/shopspring/decimal"
)

type Client struct{ http *http.Client }

func NewClient() *Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second, MaxIdleConns: 8, IdleConnTimeout: time.Minute}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, candidate := range ips {
			ip := candidate.IP
			if !publicIP(ip) {
				continue
			}
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, errors.New("exchange rate source address unavailable")
	}
	return &Client{http: &http.Client{Timeout: 10 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func publicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64 {
		return false
	}
	return true
}

func (c *Client) Fetch(ctx context.Context, source settingscontract.ExchangeRateSource, currency string) (settingscontract.ExchangeRateQuote, error) {
	quote := settingscontract.ExchangeRateQuote{ID: source.ID}
	u, err := url.Parse(source.URL)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return quote, settingscontract.ErrExchangeRateInvalid
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return quote, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return quote, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return quote, errors.New("exchange rate source HTTP error")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 512*1024+1))
	if err != nil {
		return quote, err
	}
	if len(data) > 512*1024 {
		return quote, errors.New("exchange rate response too large")
	}
	return parseQuote(source.ID, currency, data)
}

func parseQuote(id, currency string, data []byte) (settingscontract.ExchangeRateQuote, error) {
	quote := settingscontract.ExchangeRateQuote{ID: id}
	var payload struct {
		Base       string                     `json:"base"`
		BaseCode   string                     `json:"base_code"`
		Result     string                     `json:"result"`
		Date       string                     `json:"date"`
		Timestamp  int64                      `json:"timestamp"`
		LastUpdate int64                      `json:"time_last_update_unix"`
		Rates      map[string]decimal.Decimal `json:"rates"`
		USD        map[string]decimal.Decimal `json:"usd"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return quote, err
	}
	switch id {
	case "frankfurter":
		if payload.Base != "USD" {
			return quote, settingscontract.ErrExchangeRateUnavailable
		}
		quote.Rate = payload.Rates[currency]
		quote.PublishedAt, _ = time.Parse("2006-01-02", payload.Date)
	case "open_er":
		if payload.BaseCode != "USD" || payload.Result != "success" {
			return quote, settingscontract.ErrExchangeRateUnavailable
		}
		quote.Rate = payload.Rates[currency]
		quote.PublishedAt = time.Unix(payload.LastUpdate, 0).UTC()
	case "currency_api":
		quote.Rate = payload.USD[strings.ToLower(currency)]
		quote.PublishedAt, _ = time.Parse("2006-01-02", payload.Date)
	case "exchangerate_fun":
		if payload.Base != "USD" {
			return quote, settingscontract.ErrExchangeRateUnavailable
		}
		quote.Rate = payload.Rates[currency]
		quote.PublishedAt = time.Unix(payload.Timestamp, 0).UTC()
	default:
		return quote, settingscontract.ErrExchangeRateInvalid
	}
	if !quote.Rate.IsPositive() || quote.PublishedAt.IsZero() {
		return quote, settingscontract.ErrExchangeRateUnavailable
	}
	return quote, nil
}
