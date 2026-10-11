package siteconnection_test

import (
	"encoding/json"
	"fmt"
	"testing"

	siteconnectionapp "github.com/dujiao-next/internal/modules/siteconnection/application"
	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"
	siteconnectionstore "github.com/dujiao-next/internal/modules/siteconnection/infrastructure/gormstore"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestWholesaleSyncCreateAndUpdatePersistExplicitFalse(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		want        bool
	}{
		{"omitted", "", true}, {"enabled", `,"sync_wholesale_price":true`, true}, {"disabled", `,"sync_wholesale_price":false`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			sqlDB.SetMaxOpenConns(1)
			if err := db.AutoMigrate(&siteconnectiondomain.Connection{}); err != nil {
				t.Fatal(err)
			}
			store := siteconnectionstore.New(db)
			svc := siteconnectionapp.NewService(store, "test-secret", t.TempDir())
			var input siteconnectionapp.CreateInput
			if err := json.Unmarshal([]byte(fmt.Sprintf(`{"name":"test","base_url":"https://example.test","api_key":"key","api_secret":"secret"%s}`, tc.field)), &input); err != nil {
				t.Fatal(err)
			}
			conn, err := svc.Create(input)
			if err != nil {
				t.Fatal(err)
			}
			assertState := func(want bool) {
				t.Helper()
				got, err := store.GetByID(conn.ID)
				if err != nil {
					t.Fatal(err)
				}
				if got.SyncWholesalePrice != want {
					t.Fatalf("persisted wholesale sync = %v, want %v", got.SyncWholesalePrice, want)
				}
			}
			if conn.SyncWholesalePrice != tc.want {
				t.Fatalf("create response = %v, want %v", conn.SyncWholesalePrice, tc.want)
			}
			assertState(tc.want)
			for _, enabled := range []bool{false, true, false} {
				if _, err := svc.Update(conn.ID, siteconnectionapp.UpdateInput{SyncWholesalePrice: &enabled}); err != nil {
					t.Fatal(err)
				}
				assertState(enabled)
				if _, err := svc.Update(conn.ID, siteconnectionapp.UpdateInput{Name: "renamed"}); err != nil {
					t.Fatal(err)
				}
				assertState(enabled)
			}
		})
	}
}
