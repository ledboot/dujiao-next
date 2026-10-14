// Package domainhosting coordinates external domain provisioning outside database transactions.
package domainhosting

import (
	"context"
	"errors"
	"time"

	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/config"
	contract "github.com/dujiao-next/internal/modules/reseller/contract"
	domain "github.com/dujiao-next/internal/modules/reseller/domain"
)

type Provider interface {
	Reconcile(context.Context, string, domain.DomainHosting, bool) (domain.DomainHosting, error)
}

type Store interface {
	contract.ManagementStore
	ClaimDomainSync(uint, time.Time, time.Time) (bool, error)
	DueDomainIDs(time.Time, int) ([]uint, error)
}

type Service struct {
	store    Store
	provider Provider
	cfg      config.CloudflareSaaSConfig
}

func New(store Store, provider Provider, cfg config.CloudflareSaaSConfig) *Service {
	return &Service{store: store, provider: provider, cfg: cfg}
}

func (s *Service) Enabled() bool { return s != nil && s.cfg.Enabled }

// Sync uses a database lease across API processes/workers. Provider calls have a shorter
// deadline than the lease. Re-reading under lock prevents a concurrent disable from being undone.
func (s *Service) Sync(ctx context.Context, id uint) (*domain.Domain, error) {
	row, err := s.store.GetDomainByID(id)
	if err != nil || row == nil {
		return row, err
	}
	if !s.Enabled() || !row.Hosting.Managed {
		return row, nil
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	lease := now.Add(2 * time.Minute)
	claimed, err := s.store.ClaimDomainSync(id, now, lease)
	if err != nil || !claimed {
		return row, err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	// Refresh after acquiring the lease, including a possible concurrent disable.
	row, err = s.store.GetDomainByID(id)
	if err != nil || row == nil {
		return row, err
	}
	disabled := row.Status == domain.DomainStatusDisabled
	state, providerErr := s.provider.Reconcile(ctx, row.Domain, row.Hosting, disabled)
	state.Managed = true
	state.CheckedAt = &now
	state.LeaseUntil = nil
	next := now.Add(5 * time.Minute)
	state.NextCheckAt = &next
	if providerErr != nil && state.ErrorKey == "" {
		state.ErrorKey = "provider"
	}
	err = s.store.WithinManagementTransaction(func(tx contract.ManagementStore) error {
		current, err := tx.GetDomainByIDForUpdate(id)
		if err != nil || current == nil {
			return err
		}
		if current.Hosting.LeaseUntil == nil || !current.Hosting.LeaseUntil.Equal(lease) {
			return nil
		}
		state.Approved = current.Hosting.Approved
		state.RemovalRequested = current.Hosting.RemovalRequested
		current.Hosting = state
		current.VerificationStatus = domain.DomainVerificationPending
		current.VerifiedAt = nil
		if providerErr == nil && state.HostnameStatus == "active" && state.SSLStatus == "active" && state.DNSReady && current.Status != domain.DomainStatusDisabled {
			current.VerificationStatus = domain.DomainVerificationVerified
			current.VerifiedAt = &now
			if state.Approved && current.Status == domain.DomainStatusPendingReview {
				current.Status = domain.DomainStatusActive
				domains, err := tx.ListDomainsByResellerID(current.ResellerID)
				if err != nil {
					return err
				}
				primary := false
				for _, other := range domains {
					if other.ID != current.ID && other.IsPrimary && other.Status == domain.DomainStatusActive && other.VerificationStatus == domain.DomainVerificationVerified {
						primary = true
					}
				}
				if !primary {
					current.IsPrimary = true
				}
			}
		} else if state.ErrorKey != "" {
			current.VerificationStatus = domain.DomainVerificationFailed
		}
		if disabled && providerErr == nil && state.HostnameStatus == "deleted" && current.Hosting.RemovalRequested {
			current.DeletedAt = &now
		}
		// Remote cleanup is retried even if disabled during an in-flight create.
		if current.Status == domain.DomainStatusDisabled && !disabled {
			current.Hosting.NextCheckAt = &now
		}
		return tx.UpdateDomain(current)
	})
	_ = cache.DelResellerDomain(context.WithoutCancel(ctx), row.Domain)
	if err != nil {
		return nil, err
	}
	return s.store.GetDomainByID(id)
}

func (s *Service) SyncDue(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	ids, err := s.store.DueDomainIDs(time.Now().UTC().Truncate(time.Microsecond), 100)
	if err != nil {
		return err
	}
	var result error
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, err := s.Sync(ctx, id)
		result = errors.Join(result, err)
	}
	return result
}
