package gormstore

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	orderriskdomain "github.com/dujiao-next/internal/modules/orderrisk/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupRiskGateDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "risk-gate.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&orderdomain.Order{}, &orderdomain.OrderItem{}, &orderriskdomain.LockKey{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(10)
	return db
}

func TestRiskGateCountsOnlyMatchingPendingIdentityAndProducts(t *testing.T) {
	db := setupRiskGateDB(t)
	store := New(db, "risk-gate-test-secret")
	riskIP := "1.2.3.4"

	guestParent := orderdomain.Order{OrderNo: "guest-parent", UserID: 0, Status: constants.OrderStatusPendingPayment, Currency: "USD", RiskIP: riskIP}
	if err := db.Create(&guestParent).Error; err != nil {
		t.Fatal(err)
	}
	guestChild := orderdomain.Order{OrderNo: "guest-child", ParentID: &guestParent.ID, UserID: 0, Status: constants.OrderStatusPendingPayment, Currency: "USD", RiskIP: riskIP}
	if err := db.Create(&guestChild).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&orderdomain.OrderItem{OrderID: guestChild.ID, ProductID: 7, TitleJSON: jsonmap.JSON{}, Quantity: 2}).Error; err != nil {
		t.Fatal(err)
	}
	for _, order := range []orderdomain.Order{
		{OrderNo: "member-parent", UserID: 9, Status: constants.OrderStatusPendingPayment, Currency: "USD", RiskIP: riskIP},
		{OrderNo: "canceled-guest", UserID: 0, Status: constants.OrderStatusCanceled, Currency: "USD", RiskIP: riskIP},
		{OrderNo: "other-ip", UserID: 0, Status: constants.OrderStatusPendingPayment, Currency: "USD", RiskIP: "5.6.7.8"},
	} {
		if err := db.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
	}

	guestCount, err := store.CountPendingGuestByRiskIP(riskIP)
	if err != nil || guestCount != 1 {
		t.Fatalf("guest count=%d err=%v", guestCount, err)
	}
	memberCount, err := store.CountPendingMemberByRiskIP(riskIP)
	if err != nil || memberCount != 1 {
		t.Fatalf("member count=%d err=%v", memberCount, err)
	}
	quantities, err := store.SumPendingGuestQuantityByRiskIP(riskIP, []uint{7, 8})
	if err != nil || quantities[7] != 2 || quantities[8] != 0 {
		t.Fatalf("quantities=%v err=%v", quantities, err)
	}
}

func TestRiskGateSerializesConcurrentGuestQuotaChecks(t *testing.T) {
	db := setupRiskGateDB(t)
	store := New(db, "risk-gate-test-secret")
	const limit = int64(2)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	errorsCh := make(chan error, 12)
	start := make(chan struct{})

	for index := 0; index < 12; index++ {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := store.db.Transaction(func(dbtx *gorm.DB) error {
				txStore := &Store{db: dbtx, guestCredentialSecret: store.guestCredentialSecret}
				if err := txStore.LockRiskKeys([]string{"guest:ip:1.2.3.4"}); err != nil {
					return err
				}
				count, err := txStore.CountPendingGuestByRiskIP("1.2.3.4")
				if err != nil {
					return err
				}
				if count >= limit {
					return errQuotaReached
				}
				time.Sleep(5 * time.Millisecond)
				order := &orderdomain.Order{
					OrderNo:  fmt.Sprintf("concurrent-%d", index),
					UserID:   0,
					Status:   constants.OrderStatusPendingPayment,
					Currency: "USD",
					RiskIP:   "1.2.3.4",
				}
				if err := txStore.Create(order, nil); err != nil {
					return err
				}
				allowed.Add(1)
				return nil
			})
			if err != nil && !errors.Is(err, errQuotaReached) {
				errorsCh <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Fatalf("unexpected concurrent transaction error: %v", err)
	}
	if allowed.Load() != limit {
		t.Fatalf("allowed=%d, want %d", allowed.Load(), limit)
	}
	count, err := store.CountPendingGuestByRiskIP("1.2.3.4")
	if err != nil || count != limit {
		t.Fatalf("persisted count=%d err=%v", count, err)
	}
	var locks []orderriskdomain.LockKey
	if err := db.Find(&locks).Error; err != nil {
		t.Fatal(err)
	}
	if len(locks) != 1 || locks[0].KeyHash == "guest:ip:1.2.3.4" {
		t.Fatalf("expected one hashed lock key, got %#v", locks)
	}
}

var errQuotaReached = errors.New("quota reached")

func TestPendingProductMatchesIdentityAndOrderLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name      string
		userID    uint
		productID uint
		parent    string
		child     string
		deleted   string
		legacy    bool
		paid      bool
		want      bool
	}{
		{name: "pending", userID: 9, productID: 7, want: true},
		{name: "another user", userID: 10, productID: 7},
		{name: "another product", userID: 9, productID: 8},
		{name: "guest", productID: 7},
		{name: "paid", userID: 9, productID: 7, parent: constants.OrderStatusPaid, paid: true},
		{name: "canceled", userID: 9, productID: 7, parent: constants.OrderStatusCanceled},
		{name: "canceled child", userID: 9, productID: 7, child: constants.OrderStatusCanceled},
		{name: "deleted parent", userID: 9, productID: 7, deleted: "parent"},
		{name: "deleted child", userID: 9, productID: 7, deleted: "child"},
		{name: "deleted item", userID: 9, productID: 7, deleted: "item"},
		{name: "legacy order with direct items", userID: 9, productID: 7, legacy: true, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupRiskGateDB(t)
			now := time.Now()
			expired := now.Add(-time.Minute)
			parentStatus, childStatus := tc.parent, tc.child
			if parentStatus == "" {
				parentStatus = constants.OrderStatusPendingPayment
			}
			if childStatus == "" {
				childStatus = constants.OrderStatusPendingPayment
			}
			parent := orderdomain.Order{OrderNo: "parent", UserID: 9, Status: parentStatus, ExpiresAt: &expired}
			if tc.paid {
				parent.PaidAt = &now
			}
			if tc.deleted == "parent" {
				parent.DeletedAt = &now
			}
			if err := db.Create(&parent).Error; err != nil {
				t.Fatal(err)
			}
			itemOrderID := parent.ID
			if !tc.legacy {
				child := orderdomain.Order{OrderNo: "child", UserID: 9, ParentID: &parent.ID, Status: childStatus}
				if tc.deleted == "child" {
					child.DeletedAt = &now
				}
				if err := db.Create(&child).Error; err != nil {
					t.Fatal(err)
				}
				itemOrderID = child.ID
			}
			item := orderdomain.OrderItem{OrderID: itemOrderID, ProductID: 7, SKUID: 99, Quantity: 1, TitleJSON: jsonmap.JSON{}}
			if tc.deleted == "item" {
				item.DeletedAt = &now
			}
			if err := db.Create(&item).Error; err != nil {
				t.Fatal(err)
			}
			got, err := New(db, "pending-product-secret").HasPendingProductByUserID(tc.userID, []uint{tc.productID})
			if err != nil || got != tc.want {
				t.Fatalf("pending=%v want=%v err=%v", got, tc.want, err)
			}
		})
	}
}
