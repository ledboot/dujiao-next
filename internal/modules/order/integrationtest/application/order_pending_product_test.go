package application_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	cardsecretdomain "github.com/dujiao-next/internal/modules/cardsecret/domain"
	productgormstore "github.com/dujiao-next/internal/modules/catalog/product/store/gormstore"
	usergormstore "github.com/dujiao-next/internal/modules/identity/user/infrastructure/gormstore"
	orderapp "github.com/dujiao-next/internal/modules/order/application"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	ordergormstore "github.com/dujiao-next/internal/modules/order/infrastructure/gormstore"
	orderriskapp "github.com/dujiao-next/internal/modules/orderrisk/application"
	orderriskcontract "github.com/dujiao-next/internal/modules/orderrisk/contract"
	orderriskdomain "github.com/dujiao-next/internal/modules/orderrisk/domain"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	promotiongormstore "github.com/dujiao-next/internal/modules/promotion/infrastructure/gormstore"
	settingssecurity "github.com/dujiao-next/internal/modules/settings/schema/security"
)

func pendingProductFixture(t *testing.T, configs ...settingssecurity.OrderRiskControlConfig) (couponLimitRaceFixture, *orderapp.OrderService) {
	t.Helper()
	f := newCouponLimitRaceFixture(t, "pending_product", 100, 100)
	if err := f.db.AutoMigrate(&orderriskdomain.LockKey{}, &paymentdomain.Payment{}, &cardsecretdomain.Secret{}); err != nil {
		t.Fatal(err)
	}
	riskOptions := orderriskapp.Options{}
	if len(configs) > 0 {
		riskOptions.Settings = cancellationTestSettings{configs[0]}
	}
	svc := orderapp.NewOrderService(orderapp.OrderServiceOptions{
		OrderStore: ordergormstore.New(f.db, "test-guest-credential-secret-with-32-bytes"),
		UserStore:  usergormstore.New(f.db), ProductStore: productgormstore.NewProductStore(f.db),
		ProductSKUStore: productgormstore.NewSKUStore(f.db),
		CouponStore:     f.couponRepo, CouponUsageStore: f.usageRepo,
		PromotionRepo: promotiongormstore.New(f.db), Queue: f.queue, ExpireMinutes: 15,
		// No optional risk policies or Redis limiter: deduplication still applies.
		RiskControlService: orderriskapp.NewService(riskOptions),
	})
	return f, svc
}

func TestPendingProductOrderBlocksOtherSKUAndAllowsAfterCancellation(t *testing.T) {
	f, svc := pendingProductFixture(t)
	input := f.input(f.users[0].ID)
	input.CouponCode = ""
	first, err := svc.CreateOrder(input)
	if err != nil {
		t.Fatal(err)
	}
	otherSKU := f.sku
	otherSKU.ID, otherSKU.SKUCode = 0, "other-sku"
	if err := f.db.Create(&otherSKU).Error; err != nil {
		t.Fatal(err)
	}
	input.Items[0].SKUID = otherSKU.ID
	input.SkipIPRiskControl = true
	if _, err := svc.CreateOrder(input); !errors.Is(err, orderriskcontract.ErrPendingProductOrder) {
		t.Fatalf("different SKU must not bypass product restriction: %v", err)
	}
	input.UserID = f.users[1].ID
	if _, err := svc.CreateOrder(input); err != nil {
		t.Fatalf("another user must be allowed: %v", err)
	}
	if _, err := svc.CancelOrder(first.ID, f.users[0].ID); err != nil {
		t.Fatal(err)
	}
	input.UserID = f.users[0].ID
	if _, err := svc.CreateOrder(input); err != nil {
		t.Fatalf("cancellation must allow a new order: %v", err)
	}
}

func TestConcurrentPendingProductCreatesExactlyOneOrder(t *testing.T) {
	f, svc := pendingProductFixture(t)
	const requests = 8
	start := make(chan struct{})
	results := make(chan error, requests)
	var wg sync.WaitGroup
	for range requests {
		wg.Go(func() {
			<-start
			input := f.input(f.users[0].ID)
			input.CouponCode = ""
			_, err := svc.CreateOrder(input)
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	created, rejected := 0, 0
	for err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, orderriskcontract.ErrPendingProductOrder):
			rejected++
		default:
			t.Fatalf("unexpected create error: %v", err)
		}
	}
	if created != 1 || rejected != requests-1 {
		t.Fatalf("created=%d rejected=%d", created, rejected)
	}
	var count int64
	if err := f.db.Model(&orderdomain.Order{}).Where("parent_id IS NULL AND status = ?", constants.OrderStatusPendingPayment).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 || f.queue.enqueued.Load() != 1 {
		t.Fatalf("pending parents=%d timeout tasks=%d", count, f.queue.enqueued.Load())
	}
}

type cancellationTestSettings struct {
	cfg settingssecurity.OrderRiskControlConfig
}

func (s cancellationTestSettings) GetOrderRiskControlConfig() (settingssecurity.OrderRiskControlConfig, error) {
	return s.cfg, nil
}

func TestCancellationCooldownCountsUserAndTimeoutOrders(t *testing.T) {
	cfg := settingssecurity.DefaultOrderRiskControlConfig()
	cfg.Enabled = true
	cfg.Member.CancellationCooldown.Enabled = true
	f, svc := pendingProductFixture(t, cfg)
	input := f.input(f.users[0].ID)
	input.CouponCode = ""
	// Administrative cancellations must not consume the user's allowance.
	admin, err := svc.CreateOrder(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.UpdateOrderStatus(admin.ID, constants.OrderStatusCanceled); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		order, err := svc.CreateOrder(input)
		if err != nil {
			t.Fatalf("create before threshold %d: %v", i, err)
		}
		if i == 1 {
			if err = f.db.Model(&orderdomain.Order{}).Where("id = ?", order.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
				t.Fatal(err)
			}
			_, err = svc.CancelExpiredOrder(order.ID)
		} else {
			_, err = svc.CancelOrder(order.ID, input.UserID)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.CreateOrder(input); !errors.Is(err, orderriskcontract.ErrOrderCooldown) || orderriskcontract.GetRetryAfter(err) < 599 {
		t.Fatalf("expected cancellation cooldown, got %v", err)
	}
}
