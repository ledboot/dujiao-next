package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	mappingcontract "github.com/dujiao-next/internal/modules/catalog/mapping/contract"
	mappingdomain "github.com/dujiao-next/internal/modules/catalog/mapping/domain"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	settingsintegration "github.com/dujiao-next/internal/modules/settings/schema/integration"
	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/dujiao-next/internal/upstream"
	"github.com/shopspring/decimal"
)

// fakeSettingsProvider 以固定同步间隔实现 SettingsProvider。
type fakeSettingsProvider struct {
	interval time.Duration
}

func (f fakeSettingsProvider) GetUpstreamSyncConfig(fallbackInterval string) (settingsintegration.UpstreamSyncConfig, error) {
	cfg := settingsintegration.DefaultUpstreamSyncConfig()
	cfg.IntervalMinutes = int(f.interval / time.Minute)
	return cfg, nil
}

func (f fakeSettingsProvider) GetUpstreamSyncInterval(fallbackInterval string) (time.Duration, error) {
	return f.interval, nil
}

func TestComputeFullSyncIntervalFloorsAt24h(t *testing.T) {
	svc := &Service{settings: fakeSettingsProvider{interval: 5 * time.Minute}}

	// 5m 同步间隔 × 3 = 15m < 24h，期望落到 24h floor
	got := svc.computeFullSyncInterval()
	if got != fullSyncIntervalFloor {
		t.Fatalf("expected floor=24h, got %v", got)
	}
}

func TestComputeFullSyncIntervalScalesWithLongInterval(t *testing.T) {
	svc := &Service{settings: fakeSettingsProvider{interval: 12 * time.Hour}}

	// 12h * 3 = 36h，应使用 scaled 值
	got := svc.computeFullSyncInterval()
	want := 36 * time.Hour
	if got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestComputeFullSyncIntervalWithoutSettings(t *testing.T) {
	svc := &Service{}
	got := svc.computeFullSyncInterval()
	if got != fullSyncIntervalFloor {
		t.Fatalf("expected floor when settings=nil, got %v", got)
	}
}

// These stores copy records so assertions verify writes rather than pointer mutations.
type syncProductStore struct {
	mappingcontract.ProductRepository
	product productdomain.Product
}

func (s *syncProductStore) GetByID(string) (*productdomain.Product, error) {
	product := s.product
	return &product, nil
}

func (s *syncProductStore) Update(product *productdomain.Product) error {
	s.product = *product
	return nil
}

type syncSKUStore struct {
	mappingcontract.SKURepository
	skus []productdomain.ProductSKU
}

func (s *syncSKUStore) GetByID(id uint) (*productdomain.ProductSKU, error) {
	for _, sku := range s.skus {
		if sku.ID == id {
			return &sku, nil
		}
	}
	return nil, nil
}

func (s *syncSKUStore) Update(sku *productdomain.ProductSKU) error {
	for i := range s.skus {
		if s.skus[i].ID == sku.ID {
			s.skus[i] = *sku
			return nil
		}
	}
	return fmt.Errorf("SKU %d not found", sku.ID)
}

func (s *syncSKUStore) ListByProduct(productID uint, onlyActive bool) ([]productdomain.ProductSKU, error) {
	var result []productdomain.ProductSKU
	for _, sku := range s.skus {
		if sku.ProductID == productID && (!onlyActive || sku.IsActive) {
			result = append(result, sku)
		}
	}
	return result, nil
}

type syncMappingStore struct {
	mappingcontract.MappingRepository
	mapping mappingdomain.Mapping
}

func (s *syncMappingStore) GetByID(uint) (*mappingdomain.Mapping, error) {
	mapping := s.mapping
	return &mapping, nil
}

func (s *syncMappingStore) Update(mapping *mappingdomain.Mapping) error {
	s.mapping = *mapping
	return nil
}

type syncSKUMappingStore struct {
	mappingcontract.SKUMappingRepository
	mappings []mappingdomain.SKUMapping
}

func (s *syncSKUMappingStore) ListByProductMapping(uint) ([]mappingdomain.SKUMapping, error) {
	return append([]mappingdomain.SKUMapping(nil), s.mappings...), nil
}

func (s *syncSKUMappingStore) Update(mapping *mappingdomain.SKUMapping) error {
	for i := range s.mappings {
		if s.mappings[i].ID == mapping.ID {
			s.mappings[i] = *mapping
			return nil
		}
	}
	return fmt.Errorf("SKU mapping %d not found", mapping.ID)
}

type syncConnectionProvider struct {
	conn    siteconnectiondomain.Connection
	adapter upstream.Adapter
}

func (p *syncConnectionProvider) GetByID(uint) (*siteconnectiondomain.Connection, error) {
	return &p.conn, nil
}
func (p *syncConnectionProvider) GetAdapter(*siteconnectiondomain.Connection) (upstream.Adapter, error) {
	return p.adapter, nil
}

type syncProductAdapter struct {
	upstream.Adapter
	product upstream.UpstreamProduct
}

func (a *syncProductAdapter) GetProduct(context.Context, uint) (*upstream.UpstreamProduct, error) {
	return &a.product, nil
}

func TestSyncProductUpdatesCostIndependentlyOfAutoSyncPrice(t *testing.T) {
	for _, fromData := range []bool{false, true} {
		for _, autoSyncPrice := range []bool{false, true} {
			t.Run(fmt.Sprintf("fromData=%t/autoSyncPrice=%t", fromData, autoSyncPrice), func(t *testing.T) {
				amount := func(v int64) money.Amount { return money.FromDecimal(decimal.NewFromInt(v)) }
				products := &syncProductStore{product: productdomain.Product{ID: 1, PriceAmount: amount(100), CostPriceAmount: amount(10)}}
				skus := &syncSKUStore{skus: []productdomain.ProductSKU{
					{ID: 11, ProductID: 1, PriceAmount: amount(100), CostPriceAmount: amount(10), IsActive: true},
					{ID: 12, ProductID: 1, PriceAmount: amount(120), CostPriceAmount: amount(20), IsActive: true},
					{ID: 13, ProductID: 1, PriceAmount: amount(1), CostPriceAmount: amount(1), IsActive: true},
				}}
				mappings := &syncMappingStore{mapping: mappingdomain.Mapping{ID: 1, ConnectionID: 1, LocalProductID: 1, UpstreamProductID: 101}}
				skuMappings := &syncSKUMappingStore{mappings: []mappingdomain.SKUMapping{
					{ID: 1, ProductMappingID: 1, LocalSKUID: 11, UpstreamSKUID: 201, UpstreamPrice: amount(10)},
					{ID: 2, ProductMappingID: 1, LocalSKUID: 12, UpstreamSKUID: 202, UpstreamPrice: amount(20)},
					{ID: 3, ProductMappingID: 1, LocalSKUID: 13, UpstreamSKUID: 203, UpstreamPrice: amount(1)},
				}}
				upProduct := upstream.UpstreamProduct{ID: 101, IsActive: true, SKUs: []upstream.UpstreamSKU{
					{ID: 201, PriceAmount: "30.00", IsActive: true},
					{ID: 202, PriceAmount: "25.00", IsActive: true},
					{ID: 203, PriceAmount: "1.00", IsActive: false},
				}}
				connections := &syncConnectionProvider{
					conn:    siteconnectiondomain.Connection{AutoSyncPrice: autoSyncPrice, ExchangeRate: decimal.NewFromInt(2), PriceMarkupPercent: decimal.NewFromInt(50)},
					adapter: &syncProductAdapter{product: upProduct},
				}
				svc := &Service{products: products, skus: skus, mappings: mappings, skuMappings: skuMappings, connections: connections}
				if fromData {
					now := time.Now()
					svc.syncProductFromData(&mappings.mapping, &connections.conn, &upProduct, &now)
				} else if err := svc.SyncProduct(1); err != nil {
					t.Fatal(err)
				}

				wantPrices := []int64{100, 120, 1}
				wantProductPrice := int64(100)
				if autoSyncPrice {
					wantPrices = []int64{90, 75, 3}
					wantProductPrice = 75
				}
				for i, wantCost := range []int64{60, 50, 2} {
					sku := skus.skus[i]
					if !sku.CostPriceAmount.Equal(amount(wantCost).Decimal) || !sku.PriceAmount.Equal(amount(wantPrices[i]).Decimal) {
						t.Fatalf("SKU %d cost/price = %s/%s, want %d/%d", sku.ID, sku.CostPriceAmount, sku.PriceAmount, wantCost, wantPrices[i])
					}
				}
				if !products.product.CostPriceAmount.Equal(decimal.NewFromInt(50)) || !products.product.PriceAmount.Equal(decimal.NewFromInt(wantProductPrice)) {
					t.Fatalf("product cost/price = %s/%s, want 50/%d", products.product.CostPriceAmount, products.product.PriceAmount, wantProductPrice)
				}
				if skus.skus[2].IsActive {
					t.Fatal("inactive upstream SKU must be excluded from product prices")
				}
				if !skuMappings.mappings[0].UpstreamPrice.Equal(decimal.NewFromInt(30)) {
					t.Fatal("upstream price snapshot was not updated")
				}
			})
		}
	}
}
