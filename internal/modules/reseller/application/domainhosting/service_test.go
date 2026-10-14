package domainhosting_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dujiao-next/internal/config"
	app "github.com/dujiao-next/internal/modules/reseller/application"
	"github.com/dujiao-next/internal/modules/reseller/application/domainhosting"
	domain "github.com/dujiao-next/internal/modules/reseller/domain"
	store "github.com/dujiao-next/internal/modules/reseller/infrastructure/gormstore"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type provider struct {
	calls int
	hook  func()
	fail  bool
}

func (p *provider) Reconcile(_ context.Context, _ string, s domain.DomainHosting, remove bool) (domain.DomainHosting, error) {
	p.calls++
	if p.hook != nil {
		p.hook()
	}
	if p.fail {
		return s, fmt.Errorf("provider unavailable")
	}
	if remove {
		s.HostnameStatus = "deleted"
		s.HostnameID = ""
		s.DNSReady = false
		return s, nil
	}
	s.HostnameID = "remote"
	s.HostnameStatus = "active"
	s.SSLStatus = "active"
	s.DNSReady = true
	s.ErrorKey = ""
	return s, nil
}
func setup(t *testing.T) (*gorm.DB, *store.Store, *domain.Domain, *app.ManagementService, *domainhosting.Service, *provider) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := store.New(db)
	profile := domain.Profile{UserID: 17, Status: domain.ProfileStatusActive}
	if err := repo.CreateProfile(&profile); err != nil {
		t.Fatal(err)
	}
	cfg := config.ResellerConfig{Enabled: true, Cloudflare: config.CloudflareSaaSConfig{Enabled: true, CNAMETarget: "connect.platform.test"}}
	management := app.NewManagementService(repo, cfg)
	p := &provider{}
	svc := domainhosting.New(repo, p, cfg.Cloudflare)
	management.Hosting = svc
	row, err := management.SubmitUserCustomDomain(17, "shop.customer.test")
	if err != nil {
		t.Fatal(err)
	}
	return db, repo, row, management, svc, p
}
func reset(t *testing.T, db *gorm.DB, id uint) {
	t.Helper()
	if err := db.Model(&domain.Domain{}).Where("id = ?", id).Update("hosting_checked_at", nil).Error; err != nil {
		t.Fatal(err)
	}
}
func TestReviewAndTechnicalReadinessAreIndependent(t *testing.T) {
	db, repo, row, m, svc, p := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	row, err := svc.Sync(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != domain.DomainStatusPendingReview || row.VerificationStatus != domain.DomainVerificationVerified {
		t.Fatalf("review bypass: %+v", row)
	}
	// Approval may happen after technical verification.
	row, err = m.ApproveDomain(ctx, 1, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != domain.DomainStatusActive {
		t.Fatal(row.Status)
	}
	reset(t, db, row.ID)
	p.fail = true
	row, err = svc.Sync(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.VerificationStatus == domain.DomainVerificationVerified {
		t.Fatal("provider failure retained verification")
	}
	active, err := repo.FindActiveVerifiedDomain(row.Domain)
	if err != nil || active != nil {
		t.Fatal("failed domain still resolves")
	}
}
func TestDisableDuringProvisionAndCleanupRetry(t *testing.T) {
	db, repo, row, m, svc, p := setup(t)
	p.hook = func() {
		p.hook = nil
		if _, err := m.DisableDomain(context.Background(), 1, row.ID); err != nil {
			t.Error(err)
		}
	}
	row, err := svc.Sync(context.Background(), row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != domain.DomainStatusDisabled || row.VerificationStatus == domain.DomainVerificationVerified {
		t.Fatal("concurrent disable undone")
	}
	reset(t, db, row.ID)
	p.fail = true
	_, err = m.RemoveUserDomain(context.Background(), 17, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	retained, _ := repo.GetDomainByID(row.ID)
	if retained == nil || !retained.Hosting.RemovalRequested {
		t.Fatal("lost cleanup intent")
	}
	reset(t, db, row.ID)
	p.fail = false
	if _, err := svc.Sync(context.Background(), row.ID); err != nil {
		t.Fatal(err)
	}
	retained, _ = repo.GetDomainByID(row.ID)
	if retained != nil {
		t.Fatal("binding not removed after remote cleanup")
	}
}
func TestDomainOwnershipAuthorizationAndLease(t *testing.T) {
	_, _, row, m, svc, p := setup(t)
	if _, err := m.RefreshUserDomain(context.Background(), 99, row.ID); err == nil {
		t.Fatal("foreign refresh allowed")
	}
	if _, err := m.RemoveUserDomain(context.Background(), 99, row.ID); err == nil {
		t.Fatal("foreign removal allowed")
	}
	if _, err := svc.Sync(context.Background(), row.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Sync(context.Background(), row.ID); err != nil {
		t.Fatal(err)
	}
	if p.calls != 1 {
		t.Fatalf("refresh not throttled: %d", p.calls)
	}
	for _, host := range []string{"localhost", "127.0.0.1", "shop.test:443", "*.test", "bad_.test", "-bad.test", "https://shop.test"} {
		if _, err := m.SubmitUserCustomDomain(17, host); err == nil {
			t.Errorf("accepted %q", host)
		}
	}
}

func TestApprovalBeforeReadinessAndResubmission(t *testing.T) {
	db, repo, row, m, svc, p := setup(t)
	p.fail = true
	row, err := m.ApproveDomain(context.Background(), 1, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != domain.DomainStatusPendingReview || !row.Hosting.Approved {
		t.Fatal("approval incorrectly bypassed failed provisioning")
	}
	reset(t, db, row.ID)
	p.fail = false
	row, err = svc.Sync(context.Background(), row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != domain.DomainStatusActive || !row.IsPrimary {
		t.Fatal("approved domain not activated after readiness")
	}
	oldToken := row.Hosting.OwnershipToken
	if _, err := m.RemoveUserDomain(context.Background(), 17, row.ID); err != nil {
		t.Fatal(err)
	}
	gone, _ := repo.GetDomainByID(row.ID)
	if gone != nil {
		t.Fatal("cleanup did not remove binding")
	}
	row, err = m.SubmitUserCustomDomain(17, "shop.customer.test")
	if err != nil {
		t.Fatal(err)
	}
	if row.Hosting.OwnershipToken == oldToken || row.Hosting.RemovalRequested || row.Hosting.Approved || row.Hosting.HostnameID != "" {
		t.Fatal("resubmission inherited old provider authorization")
	}
	if _, err := m.RefreshDomain(context.Background(), 1, 999); err == nil {
		t.Fatal("missing domain accepted")
	}
}
