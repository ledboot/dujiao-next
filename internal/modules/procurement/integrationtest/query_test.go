package procurement_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	orderdomain "github.com/dujiao-next/internal/modules/order/domain"

	"github.com/dujiao-next/internal/constants"
	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
	procurementdomain "github.com/dujiao-next/internal/modules/procurement/domain"
	procurementgormstore "github.com/dujiao-next/internal/modules/procurement/infrastructure/gormstore"
	siteconnectionapp "github.com/dujiao-next/internal/modules/siteconnection/application"
	"github.com/dujiao-next/internal/shared/money"

	"github.com/shopspring/decimal"
)

func TestProcurementStoreExcludesSoftDeletedRows(t *testing.T) {
	db := setupProcurementTestDB(t)
	order := createProcTestOrder(t, db, "PROC-SOFT-DELETED", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	proc := createTestProcurementOrder(t, db, 1, order.ID, order.OrderNo, constants.ProcurementStatusAccepted)

	deletedAt := time.Now()
	if err := db.Model(&procurementdomain.Order{}).
		Where("id = ?", proc.ID).
		Update("deleted_at", deletedAt).Error; err != nil {
		t.Fatalf("soft delete procurement order: %v", err)
	}

	svc := newTestProcurementService(db, newTestSiteConnectionService(db, "test-key", t.TempDir()))
	if _, err := svc.GetByID(proc.ID); !errors.Is(err, procurementcontract.ErrNotFound) {
		t.Fatalf("GetByID error = %v, want ErrNotFound", err)
	}
	orders, total, err := svc.List(ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 0 || len(orders) != 0 {
		t.Fatalf("List returned soft-deleted row: total=%d len=%d", total, len(orders))
	}
	stats, err := svc.StatsByStatus(ListFilter{})
	if err != nil {
		t.Fatalf("StatsByStatus: %v", err)
	}
	if stats[constants.ProcurementStatusAccepted] != 0 {
		t.Fatalf("StatsByStatus counted soft-deleted row: %v", stats)
	}
}

func TestProcurement_GetByID_DoesNotIncludeLocalRefundRecords(t *testing.T) {
	db := setupProcurementTestDB(t)

	parent := createProcTestOrder(t, db, "PROC-PARENT-001", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	child := createProcTestOrder(t, db, "PROC-CHILD-001", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	if err := db.Model(&child).Update("parent_id", parent.ID).Error; err != nil {
		t.Fatalf("set child parent: %v", err)
	}

	proc := createTestProcurementOrder(t, db, 1, child.ID, child.OrderNo, constants.ProcurementStatusAccepted)

	localRecord := &orderdomain.OrderRefundRecord{
		OrderID:    child.ID,
		Type:       constants.OrderRefundTypeManual,
		Amount:     money.FromDecimal(decimal.NewFromFloat(10.5)),
		Currency:   "CNY",
		Remark:     "local refund",
		GuestEmail: "guest-local@example.com",
	}
	if err := db.Create(localRecord).Error; err != nil {
		t.Fatalf("create local refund record: %v", err)
	}

	parentRecord := &orderdomain.OrderRefundRecord{
		OrderID:    parent.ID,
		Type:       constants.OrderRefundTypeWallet,
		Amount:     money.FromDecimal(decimal.NewFromFloat(7.25)),
		Currency:   "CNY",
		Remark:     "parent refund",
		GuestEmail: "guest-parent@example.com",
	}
	if err := db.Create(parentRecord).Error; err != nil {
		t.Fatalf("create parent refund record: %v", err)
	}

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	svc := newTestProcurementService(db, connSvc)

	got, err := svc.GetByID(proc.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected procurement order")
	}
	if got.UpstreamRefundedAmount != "" || len(got.UpstreamRefundRecords) != 0 {
		t.Fatalf("expected no upstream refund fields, got refunded_amount=%q records=%d", got.UpstreamRefundedAmount, len(got.UpstreamRefundRecords))
	}
}

func TestProcurement_FillParentOrderNo_BackfillsLocalRefundedAmountFromParent(t *testing.T) {
	db := setupProcurementTestDB(t)

	parent := createProcTestOrder(t, db, "PROC-PARENT-REFUND-001", constants.OrderStatusPartiallyRefunded, constants.FulfillmentTypeUpstream)
	if err := db.Model(&orderdomain.Order{}).Where("id = ?", parent.ID).Updates(map[string]interface{}{
		"refunded_amount": money.FromDecimal(decimal.NewFromFloat(12.34)),
	}).Error; err != nil {
		t.Fatalf("set parent refunded_amount: %v", err)
	}

	child := createProcTestOrder(t, db, "PROC-CHILD-REFUND-001", constants.OrderStatusPartiallyRefunded, constants.FulfillmentTypeUpstream)
	if err := db.Model(&orderdomain.Order{}).Where("id = ?", child.ID).Update("parent_id", parent.ID).Error; err != nil {
		t.Fatalf("set child parent: %v", err)
	}

	proc := createTestProcurementOrder(t, db, 1, child.ID, child.OrderNo, constants.ProcurementStatusAccepted)
	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	svc := newTestProcurementService(db, connSvc)

	got, err := svc.GetByID(proc.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.LocalOrder == nil {
		t.Fatalf("expected procurement with local_order, got %+v", got)
	}

	svc.FillParentOrderNo(got)

	if got.ParentOrderNo != parent.OrderNo {
		t.Fatalf("expected parent_order_no %q, got %q", parent.OrderNo, got.ParentOrderNo)
	}
	if got.LocalOrder.RefundedAmount.String() != "12.34" {
		t.Fatalf("expected local_order.refunded_amount 12.34, got %s", got.LocalOrder.RefundedAmount.String())
	}
}

func TestProcurement_List_BackfillsChildLocalRefundedAmountFromParent(t *testing.T) {
	db := setupProcurementTestDB(t)

	parent := createProcTestOrder(t, db, "PROC-LIST-PARENT-REFUND-001", constants.OrderStatusPartiallyRefunded, constants.FulfillmentTypeUpstream)
	if err := db.Model(&orderdomain.Order{}).Where("id = ?", parent.ID).Updates(map[string]interface{}{
		"refunded_amount": money.FromDecimal(decimal.NewFromFloat(8.88)),
	}).Error; err != nil {
		t.Fatalf("set parent refunded_amount: %v", err)
	}

	child := createProcTestOrder(t, db, "PROC-LIST-CHILD-REFUND-001", constants.OrderStatusPartiallyRefunded, constants.FulfillmentTypeUpstream)
	if err := db.Model(&orderdomain.Order{}).Where("id = ?", child.ID).Update("parent_id", parent.ID).Error; err != nil {
		t.Fatalf("set child parent: %v", err)
	}

	proc := createTestProcurementOrder(t, db, 1, child.ID, child.OrderNo, constants.ProcurementStatusAccepted)
	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	svc := newTestProcurementService(db, connSvc)

	orders, total, err := svc.List(ListFilter{
		LocalOrderNo: child.OrderNo,
		Page:         1,
		PageSize:     20,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(orders) != 1 || orders[0].ID != proc.ID {
		t.Fatalf("unexpected procurement list result: total=%d len=%d orders=%+v", total, len(orders), orders)
	}
	if orders[0].ParentOrderNo != parent.OrderNo {
		t.Fatalf("expected parent_order_no %q, got %q", parent.OrderNo, orders[0].ParentOrderNo)
	}
	if orders[0].LocalOrder == nil {
		t.Fatalf("expected local_order in list result")
	}
	if orders[0].LocalOrder.RefundedAmount.String() != "8.88" {
		t.Fatalf("expected local_order.refunded_amount 8.88, got %s", orders[0].LocalOrder.RefundedAmount.String())
	}
}

func TestProcurement_List_DoesNotIncludeLocalRefundRecords(t *testing.T) {
	db := setupProcurementTestDB(t)

	order := createProcTestOrder(t, db, "PROC-LIST-REFUND-001", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	proc := createTestProcurementOrder(t, db, 1, order.ID, order.OrderNo, constants.ProcurementStatusAccepted)

	record := &orderdomain.OrderRefundRecord{
		OrderID:  order.ID,
		Type:     constants.OrderRefundTypeManual,
		Amount:   money.FromDecimal(decimal.NewFromInt(12)),
		Currency: "CNY",
		Remark:   "list refund",
		UserID:   1,
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create refund record: %v", err)
	}

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	svc := newTestProcurementService(db, connSvc)

	orders, total, err := svc.List(ListFilter{
		LocalOrderNo: order.OrderNo,
		Page:         1,
		PageSize:     20,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(orders) != 1 || orders[0].ID != proc.ID {
		t.Fatalf("unexpected procurement list result: %+v", orders)
	}
	if orders[0].UpstreamRefundedAmount != "" || len(orders[0].UpstreamRefundRecords) != 0 {
		t.Fatalf("expected no upstream refund fields in list result, got refunded_amount=%q records=%d", orders[0].UpstreamRefundedAmount, len(orders[0].UpstreamRefundRecords))
	}
}

func TestProcurement_GetByID_SyncsUpstreamRefundStatusAndRecords(t *testing.T) {
	db := setupProcurementTestDB(t)
	order := createProcTestOrder(t, db, "PROC-UPSTREAM-REFUND-001", constants.OrderStatusDelivered, constants.FulfillmentTypeUpstream)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"order_id":        999,
			"order_no":        "UP-999",
			"status":          "partially_refunded",
			"amount":          "50.00",
			"refunded_amount": "10.00",
			"currency":        "CNY",
			"refund_records": []map[string]any{
				{
					"id":         101,
					"type":       "manual",
					"amount":     "10.00",
					"currency":   "CNY",
					"remark":     "upstream partial refund",
					"created_at": time.Now().Format(time.RFC3339),
				},
			},
		})
	}))
	defer server.Close()

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	conn, err := connSvc.Create(siteconnectionapp.CreateInput{
		Name:      "upstream-refund",
		BaseURL:   server.URL,
		ApiKey:    "key",
		ApiSecret: "secret",
		Protocol:  constants.ConnectionProtocolDujiaoNext,
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}

	proc := createTestProcurementOrder(t, db, conn.ID, order.ID, order.OrderNo, constants.ProcurementStatusFulfilled)
	if err := db.Model(&ProcurementOrder{}).Where("id = ?", proc.ID).Updates(map[string]interface{}{
		"upstream_order_id": uint(999),
		"upstream_order_no": "UP-999",
	}).Error; err != nil {
		t.Fatalf("set upstream order info: %v", err)
	}

	svc := newTestProcurementService(db, connSvc)
	got, err := svc.GetByID(proc.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected procurement order")
	}
	if got.Status != constants.ProcurementStatusPartiallyRefunded {
		t.Fatalf("expected status %s, got %s", constants.ProcurementStatusPartiallyRefunded, got.Status)
	}
	if len(got.UpstreamRefundRecords) != 1 {
		t.Fatalf("expected 1 upstream_refund_records, got %d", len(got.UpstreamRefundRecords))
	}
	if id, ok := got.UpstreamRefundRecords[0]["id"].(int); !ok || id != 1 {
		t.Fatalf("expected upstream_refund_records[0].id = 1, got %#v", got.UpstreamRefundRecords[0]["id"])
	}
	if got.UpstreamRefundedAmount != "10.00" {
		t.Fatalf("expected upstream_refunded_amount 10.00, got %q", got.UpstreamRefundedAmount)
	}

	var refreshed ProcurementOrder
	if err := db.First(&refreshed, proc.ID).Error; err != nil {
		t.Fatalf("reload procurement order: %v", err)
	}
	if refreshed.Status != constants.ProcurementStatusPartiallyRefunded {
		t.Fatalf("expected persisted status %s, got %s", constants.ProcurementStatusPartiallyRefunded, refreshed.Status)
	}
}

func TestProcurement_List_SyncsUpstreamRefundStatusAndRecords(t *testing.T) {
	db := setupProcurementTestDB(t)
	order := createProcTestOrder(t, db, "PROC-UPSTREAM-REFUND-LIST-001", constants.OrderStatusDelivered, constants.FulfillmentTypeUpstream)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"order_id":        888,
			"order_no":        "UP-888",
			"status":          "partially_refunded",
			"amount":          "80.00",
			"refunded_amount": "8.00",
			"currency":        "CNY",
			"refund_records": []map[string]any{
				{"id": 201, "type": "wallet", "amount": "8.00", "currency": "CNY", "remark": "list upstream refund"},
			},
		})
	}))
	defer server.Close()

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	conn, err := connSvc.Create(siteconnectionapp.CreateInput{
		Name:      "upstream-refund-list",
		BaseURL:   server.URL,
		ApiKey:    "key",
		ApiSecret: "secret",
		Protocol:  constants.ConnectionProtocolDujiaoNext,
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}

	proc := createTestProcurementOrder(t, db, conn.ID, order.ID, order.OrderNo, constants.ProcurementStatusFulfilled)
	if err := db.Model(&ProcurementOrder{}).Where("id = ?", proc.ID).Updates(map[string]interface{}{
		"upstream_order_id": uint(888),
		"upstream_order_no": "UP-888",
	}).Error; err != nil {
		t.Fatalf("set upstream order info: %v", err)
	}

	svc := newTestProcurementService(db, connSvc)
	orders, total, err := svc.List(ListFilter{
		LocalOrderNo: order.OrderNo,
		Page:         1,
		PageSize:     20,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(orders) != 1 {
		t.Fatalf("unexpected list result: total=%d len=%d", total, len(orders))
	}
	if orders[0].Status != constants.ProcurementStatusPartiallyRefunded {
		t.Fatalf("expected list status %s, got %s", constants.ProcurementStatusPartiallyRefunded, orders[0].Status)
	}
	if len(orders[0].UpstreamRefundRecords) != 1 {
		t.Fatalf("expected 1 upstream_refund_records, got %d", len(orders[0].UpstreamRefundRecords))
	}
	if id, ok := orders[0].UpstreamRefundRecords[0]["id"].(int); !ok || id != 1 {
		t.Fatalf("expected upstream_refund_records[0].id = 1, got %#v", orders[0].UpstreamRefundRecords[0]["id"])
	}
	if orders[0].UpstreamRefundedAmount != "8.00" {
		t.Fatalf("expected upstream_refunded_amount 8.00, got %q", orders[0].UpstreamRefundedAmount)
	}
}

func TestProcurement_GetByID_WithoutUpstreamRefundOmitsRefundFields(t *testing.T) {
	db := setupProcurementTestDB(t)
	order := createProcTestOrder(t, db, "PROC-UPSTREAM-NO-REFUND-001", constants.OrderStatusDelivered, constants.FulfillmentTypeUpstream)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"order_id":        777,
			"order_no":        "UP-777",
			"status":          "fulfilled",
			"amount":          "66.00",
			"refunded_amount": "0.00",
			"currency":        "CNY",
			"refund_records":  []map[string]any{},
		})
	}))
	defer server.Close()

	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	conn, err := connSvc.Create(siteconnectionapp.CreateInput{
		Name:      "upstream-no-refund",
		BaseURL:   server.URL,
		ApiKey:    "key",
		ApiSecret: "secret",
		Protocol:  constants.ConnectionProtocolDujiaoNext,
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}

	proc := createTestProcurementOrder(t, db, conn.ID, order.ID, order.OrderNo, constants.ProcurementStatusFulfilled)
	if err := db.Model(&ProcurementOrder{}).Where("id = ?", proc.ID).Updates(map[string]interface{}{
		"upstream_order_id": uint(777),
		"upstream_order_no": "UP-777",
	}).Error; err != nil {
		t.Fatalf("set upstream order info: %v", err)
	}

	svc := newTestProcurementService(db, connSvc)
	got, err := svc.GetByID(proc.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected procurement order")
	}
	if got.Status != constants.ProcurementStatusFulfilled {
		t.Fatalf("expected status %s, got %s", constants.ProcurementStatusFulfilled, got.Status)
	}
	if got.UpstreamRefundedAmount != "" {
		t.Fatalf("expected empty upstream_refunded_amount, got %q", got.UpstreamRefundedAmount)
	}
	if len(got.UpstreamRefundRecords) != 0 {
		t.Fatalf("expected empty upstream_refund_records, got %d", len(got.UpstreamRefundRecords))
	}

	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal procurement order failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal procurement order payload failed: %v", err)
	}
	if _, ok := decoded["upstream_refunded_amount"]; ok {
		t.Fatalf("expected upstream_refunded_amount to be omitted when no upstream refund, payload=%s", string(payload))
	}
	if _, ok := decoded["upstream_refund_records"]; ok {
		t.Fatalf("expected upstream_refund_records to be omitted when no upstream refund, payload=%s", string(payload))
	}
	if _, ok := decoded["upstream_order_id"]; ok {
		t.Fatalf("expected upstream_order_id to be omitted from procurement payload, payload=%s", string(payload))
	}
}

func TestProcurement_ListAndStats_SupportFuzzyOrderNoAndUpstreamOrderNo(t *testing.T) {
	db := setupProcurementTestDB(t)
	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	svc := newTestProcurementService(db, connSvc)

	orderA := createProcTestOrder(t, db, "DJ20261124ALPHA9901", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	procA := createTestProcurementOrder(t, db, 1, orderA.ID, orderA.OrderNo, constants.ProcurementStatusAccepted)
	if err := db.Model(&ProcurementOrder{}).Where("id = ?", procA.ID).Updates(map[string]interface{}{
		"upstream_order_no":   "UP-20261124-XYZ7788",
		"downstream_order_no": "DJ20261124ALPHA9901-R1",
	}).Error; err != nil {
		t.Fatalf("update procA order nos: %v", err)
	}

	parentB := createProcTestOrder(t, db, "DJ20261124PARENT5566", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	childB := createProcTestOrder(t, db, "DJ20261124CHILD8811", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	if err := db.Model(&orderdomain.Order{}).Where("id = ?", childB.ID).Update("parent_id", parentB.ID).Error; err != nil {
		t.Fatalf("set childB parent_id: %v", err)
	}
	procB := createTestProcurementOrder(t, db, 1, childB.ID, childB.OrderNo, constants.ProcurementStatusPending)
	if err := db.Model(&ProcurementOrder{}).Where("id = ?", procB.ID).Updates(map[string]interface{}{
		"upstream_order_no": "UP-20261124-BETA3344",
	}).Error; err != nil {
		t.Fatalf("update procB upstream_order_no: %v", err)
	}

	cases := []struct {
		name            string
		localOrderNo    string
		upstreamOrderNo string
		wantIDs         []uint
	}{
		{
			name:         "fuzzy local_order_no lowercase substring",
			localOrderNo: "alpha99",
			wantIDs:      []uint{procA.ID},
		},
		{
			name:         "fuzzy downstream_order_no retry suffix",
			localOrderNo: "9901-r1",
			wantIDs:      []uint{procA.ID},
		},
		{
			name:         "fuzzy parent_order_no fragment matches child procurement order",
			localOrderNo: "parent5566",
			wantIDs:      []uint{procB.ID},
		},
		{
			name:            "fuzzy upstream_order_no lowercase substring",
			upstreamOrderNo: "xyz7788",
			wantIDs:         []uint{procA.ID},
		},
		{
			name:            "shared prefix matches both procurement orders",
			upstreamOrderNo: "up-20261124",
			wantIDs:         []uint{procB.ID, procA.ID},
		},
		{
			name:         "non-matching keyword returns empty",
			localOrderNo: "NO-SUCH-PROC",
			wantIDs:      nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filter := ListFilter{
				LocalOrderNo:    tc.localOrderNo,
				UpstreamOrderNo: tc.upstreamOrderNo,
				Page:            1,
				PageSize:        20,
			}
			orders, total, err := svc.List(filter)
			if err != nil {
				t.Fatalf("List(%+v): %v", filter, err)
			}
			if int(total) != len(tc.wantIDs) || len(orders) != len(tc.wantIDs) {
				t.Fatalf("List(%+v) total=%d len=%d, want %d", filter, total, len(orders), len(tc.wantIDs))
			}
			gotSet := make(map[uint]bool, len(orders))
			for _, item := range orders {
				gotSet[item.ID] = true
			}
			for _, wantID := range tc.wantIDs {
				if !gotSet[wantID] {
					t.Fatalf("List(%+v) missing expected procurement ID %d", filter, wantID)
				}
			}

			stats, err := svc.StatsByStatus(filter)
			if err != nil {
				t.Fatalf("StatsByStatus(%+v): %v", filter, err)
			}
			var statsTotal int64
			for _, v := range stats {
				statsTotal += v
			}
			if int(statsTotal) != len(tc.wantIDs) {
				t.Fatalf("StatsByStatus(%+v) sum=%d, want %d (stats=%v)", filter, statsTotal, len(tc.wantIDs), stats)
			}
		})
	}
}

func TestProcurement_ListByLocalOrderIDs_LoadsConnectionsAndSkipsSoftDeleted(t *testing.T) {
	db := setupProcurementTestDB(t)
	connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
	conn, err := connSvc.Create(siteconnectionapp.CreateInput{
		Name:      "Main Upstream",
		BaseURL:   "https://upstream.example.com",
		ApiKey:    "key",
		ApiSecret: "secret",
		Protocol:  constants.ConnectionProtocolDujiaoNext,
	})
	if err != nil {
		t.Fatalf("create connection: %v", err)
	}

	parent := createProcTestOrder(t, db, "DJ-DETAIL-PARENT", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	child1 := createProcTestOrder(t, db, "DJ-DETAIL-CHILD-1", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	child2 := createProcTestOrder(t, db, "DJ-DETAIL-CHILD-2", constants.OrderStatusPaid, constants.FulfillmentTypeUpstream)
	proc1 := createTestProcurementOrder(t, db, conn.ID, child1.ID, child1.OrderNo, constants.ProcurementStatusFulfilled)
	proc2 := createTestProcurementOrder(t, db, conn.ID, child2.ID, child2.OrderNo, constants.ProcurementStatusAccepted)
	if err := db.Model(&ProcurementOrder{}).Where("id = ?", proc1.ID).Update("upstream_order_no", "UP-CHILD-001").Error; err != nil {
		t.Fatalf("set proc1 upstream_order_no: %v", err)
	}
	deletedAt := time.Now()
	if err := db.Model(&ProcurementOrder{}).Where("id = ?", proc2.ID).Update("deleted_at", deletedAt).Error; err != nil {
		t.Fatalf("soft delete proc2: %v", err)
	}

	store := procurementgormstore.New(db)
	got, err := store.ListByLocalOrderIDs([]uint{parent.ID, child1.ID, child2.ID})
	if err != nil {
		t.Fatalf("ListByLocalOrderIDs: %v", err)
	}
	if len(got) != 1 || got[0].ID != proc1.ID {
		t.Fatalf("expected only proc1, got %+v", got)
	}
	if got[0].UpstreamOrderNo != "UP-CHILD-001" {
		t.Fatalf("expected upstream_order_no UP-CHILD-001, got %q", got[0].UpstreamOrderNo)
	}
	if got[0].Connection == nil || got[0].Connection.Name != "Main Upstream" {
		t.Fatalf("expected preloaded connection Main Upstream, got %+v", got[0].Connection)
	}
}
