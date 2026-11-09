package exchangerate

import (
	"github.com/shopspring/decimal"
	"net"
	"testing"
)

func TestParseProviderQuotes(t *testing.T) {
	for _, test := range []struct{ id, body string }{
		{"frankfurter", `{"base":"USD","date":"2026-11-09","rates":{"CNY":6.68}}`},
		{"open_er", `{"base_code":"USD","result":"success","time_last_update_unix":1794232800,"rates":{"CNY":6.68}}`},
		{"currency_api", `{"date":"2026-11-09","usd":{"cny":6.68}}`},
		{"exchangerate_fun", `{"base":"USD","timestamp":1794232800,"rates":{"CNY":6.68}}`},
	} {
		t.Run(test.id, func(t *testing.T) {
			quote, err := parseQuote(test.id, "CNY", []byte(test.body))
			if err != nil {
				t.Fatal(err)
			}
			if !quote.Rate.Equal(decimal.RequireFromString("6.68")) || quote.PublishedAt.IsZero() {
				t.Fatalf("quote: %+v", quote)
			}
		})
	}
	for _, body := range []string{`{"base":"EUR","date":"2026-11-09","rates":{"CNY":6.68}}`, `{"base":"USD","rates":{"CNY":6.68}}`, `{"base":"USD","date":"2026-11-09","rates":{"CNY":-1}}`} {
		if _, err := parseQuote("frankfurter", "CNY", []byte(body)); err == nil {
			t.Fatal("invalid quote accepted")
		}
	}
}
func TestSourceAddressesExcludePrivateNetworks(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "::1", "fc00::1", "0.0.0.0"} {
		if publicIP(net.ParseIP(ip)) {
			t.Fatalf("private address accepted: %s", ip)
		}
	}
	if !publicIP(net.ParseIP("1.1.1.1")) {
		t.Fatal("public address rejected")
	}
}
