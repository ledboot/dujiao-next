package integrationtest

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/config"
	orderriskcontract "github.com/dujiao-next/internal/modules/orderrisk/contract"
	"github.com/dujiao-next/internal/modules/orderrisk/infrastructure/redislimiter"
	settingssecurity "github.com/dujiao-next/internal/modules/settings/schema/security"
)

func TestRedisRateLimitSerializesRequestsAndKeepsCooldown(t *testing.T) {
	bin, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("redis-server is not installed")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	cmd := exec.Command(bin, "--bind", "127.0.0.1", "--port", strconv.Itoa(port), "--save", "", "--appendonly", "no")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	if err := cache.InitRedis(&config.RedisConfig{Enabled: true, Host: "127.0.0.1", Port: port}); err != nil {
		t.Fatal(err)
	}
	client := cache.Client()
	t.Cleanup(func() { _ = client.Close(); _ = cache.InitRedis(&config.RedisConfig{}) })
	limiter := redislimiter.New()
	ctx := context.Background()
	deadline := time.Now().Add(3 * time.Second)
	for client.Ping(ctx).Err() != nil {
		if time.Now().After(deadline) {
			t.Fatal("isolated Redis did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cfg := settingssecurity.OrderRateLimitConfig{WindowSeconds: 60, MaxRequests: 3, BlockSeconds: 120}
	results := make(chan error, 12)
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() { results <- limiter.Check(orderriskcontract.CheckInput{UserID: 9}, cfg) })
	}
	wg.Wait()
	close(results)
	allowed, limited := 0, 0
	for err := range results {
		switch {
		case err == nil:
			allowed++
		case errors.Is(err, orderriskcontract.ErrOrderRateLimited):
			limited++
			if n := orderriskcontract.GetRetryAfter(err); n < 119 || n > 120 {
				t.Fatalf("unexpected Retry-After %d", n)
			}
		default:
			t.Fatalf("rate limit error: %v", err)
		}
	}
	if allowed != 3 || limited != 9 {
		t.Fatalf("allowed=%d limited=%d", allowed, limited)
	}
	if err := limiter.Check(orderriskcontract.CheckInput{UserID: 10}, cfg); err != nil {
		t.Fatalf("another account blocked: %v", err)
	}
	if err := client.Expire(ctx, "dj:risk:order_rate:user:9", 30*time.Second).Err(); err != nil {
		t.Fatal(err)
	}
	err = limiter.Check(orderriskcontract.CheckInput{UserID: 9}, cfg)
	if !errors.Is(err, orderriskcontract.ErrOrderRateLimited) || orderriskcontract.GetRetryAfter(err) > 30 {
		t.Fatalf("retries extended the block: %v", err)
	}
}
