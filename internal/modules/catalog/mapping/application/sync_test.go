package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	mappingcontract "github.com/dujiao-next/internal/modules/catalog/mapping/contract"
	mappingdomain "github.com/dujiao-next/internal/modules/catalog/mapping/domain"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	settingsintegration "github.com/dujiao-next/internal/modules/settings/schema/integration"
	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
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
					svc.syncProductFromData(&mappings.mapping, &connections.conn, &upProduct, &now, nil, true)
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

// failingCategoryAdapter 让分类接口直接报错，商品接口正常返回。
type failingCategoryAdapter struct {
	upstream.Adapter
	product *upstream.UpstreamProduct
}

func (a failingCategoryAdapter) ListCategories(_ context.Context) (*upstream.CategoryListResult, error) {
	return nil, errors.New("upstream category endpoint timed out")
}

func (a failingCategoryAdapter) GetProduct(_ context.Context, _ uint) (*upstream.UpstreamProduct, error) {
	return a.product, nil
}

type staticConnectionProvider struct {
	conn    *siteconnectiondomain.Connection
	adapter upstream.Adapter
}

func (p staticConnectionProvider) GetByID(_ uint) (*siteconnectiondomain.Connection, error) {
	return p.conn, nil
}

func (p staticConnectionProvider) GetAdapter(_ *siteconnectiondomain.Connection) (upstream.Adapter, error) {
	return p.adapter, nil
}

type staticMappingRepo struct {
	mappingcontract.MappingRepository
	mapping *mappingdomain.Mapping
}

func (r staticMappingRepo) GetByID(_ uint) (*mappingdomain.Mapping, error) {
	return r.mapping, nil
}

func (r staticMappingRepo) Update(_ *mappingdomain.Mapping) error {
	return nil
}

// recordingSKURepo 记录被写回的 SKU，用来断言价格没被动过。
type recordingSKURepo struct {
	mappingcontract.SKURepository
	updates []*productdomain.ProductSKU
}

func (r *recordingSKURepo) Update(item *productdomain.ProductSKU) error {
	copied := *item
	r.updates = append(r.updates, &copied)
	return nil
}

func (r *recordingSKURepo) GetByID(_ uint) (*productdomain.ProductSKU, error) {
	return &productdomain.ProductSKU{ID: 1, PriceAmount: money.FromDecimal(decimal.NewFromInt(100))}, nil
}

// 分类接口报错时，已启用自动同步价格的商品售价必须保持不变。
// 这是修复前最危险的行为：拿不到父分类就退回连接默认加价（可能是 0%），
// 把「父分类 +50%」的商品按成本价写回数据库。
// 同时不能返回错误：库存同步与上下架判断必须照常进行（与 SyncConnectionStock 一致）。
func TestSyncProductKeepsPriceWhenCategoryTreeUnavailable(t *testing.T) {
	markups := jsonmap.JSON{"1": float64(50)}
	conn := &siteconnectiondomain.Connection{
		ID:                 1,
		ExchangeRate:       decimal.NewFromInt(1),
		PriceMarkupPercent: decimal.Zero,
		CategoryMarkups:    markups,
		AutoSyncPrice:      true,
	}
	mapping := &mappingdomain.Mapping{ID: 1, ConnectionID: 1, LocalProductID: 10, UpstreamProductID: 20}
	adapter := failingCategoryAdapter{product: &upstream.UpstreamProduct{
		ID:          20,
		CategoryID:  2, // 自身没配加价，需要向父分类 1 继承 50%
		IsActive:    true,
		PriceAmount: "10",
	}}
	skus := &recordingSKURepo{}
	svc := &Service{
		mappings:    staticMappingRepo{mapping: mapping},
		connections: staticConnectionProvider{conn: conn, adapter: adapter},
		products:    staticProductRepo{},
		skus:        skus,
		skuMappings: &recordingSKUMappingRepo{},
	}

	if err := svc.SyncProduct(1); err != nil {
		t.Fatalf("category tree failure must not abort the sync, got %v", err)
	}
	if len(skus.updates) != 0 {
		t.Fatalf("price must not be written when the category tree is unavailable, got %d SKU updates", len(skus.updates))
	}
}

// 分类接口报错时，上游已下架的商品仍要被停用——这一步不能因为拿不到分类树而被跳过。
func TestSyncProductStillMarksInactiveWhenCategoryTreeUnavailable(t *testing.T) {
	conn := &siteconnectiondomain.Connection{
		ID:                 1,
		ExchangeRate:       decimal.NewFromInt(1),
		PriceMarkupPercent: decimal.Zero,
		CategoryMarkups:    jsonmap.JSON{"1": float64(50)},
		AutoSyncPrice:      true,
	}
	mapping := &mappingdomain.Mapping{ID: 1, ConnectionID: 1, LocalProductID: 10, UpstreamProductID: 20}
	adapter := failingCategoryAdapter{product: &upstream.UpstreamProduct{
		ID:         20,
		CategoryID: 2,
		IsActive:   false,
	}}
	mappings := &recordingMappingRepo{mapping: mapping}
	svc := &Service{
		mappings:    mappings,
		connections: staticConnectionProvider{conn: conn, adapter: adapter},
		products:    staticProductRepo{},
		skus:        &recordingSKURepo{},
		skuMappings: &recordingSKUMappingRepo{},
	}

	if err := svc.SyncProduct(1); err != nil {
		t.Fatalf("inactive product must be handled even without a category tree, got %v", err)
	}
	if mapping.UpstreamStatus != mappingdomain.UpstreamStatusInactive {
		t.Fatalf("upstream status = %q, want inactive", mapping.UpstreamStatus)
	}
}

// 关闭「自动同步价格」时，上游新增的 SKU 仍要按父分类的加价建出来，
// 不能按连接默认加价（这里是 0%）写成一个错价的已上架 SKU。
func TestSyncProductNewSKUUsesParentCategoryMarkupWhenAutoSyncDisabled(t *testing.T) {
	// 父分类 1 配 50%，分类 2 没配；商品属于分类 2。
	conn := &siteconnectiondomain.Connection{
		ID:                 1,
		ExchangeRate:       decimal.NewFromInt(1),
		PriceMarkupPercent: decimal.Zero,
		CategoryMarkups:    jsonmap.JSON{"1": float64(50)},
		AutoSyncPrice:      false,
		PriceRoundingMode:  "none",
	}
	mapping := &mappingdomain.Mapping{ID: 1, ConnectionID: 1, LocalProductID: 10, UpstreamProductID: 20}
	adapter := &parentCategoryAdapter{product: &upstream.UpstreamProduct{
		ID:          20,
		CategoryID:  2,
		IsActive:    true,
		PriceAmount: "10",
		SKUs:        []upstream.UpstreamSKU{{ID: 99, SKUCode: "NEW", PriceAmount: "10", IsActive: true}},
	}}
	skus := &creatingSKURepo{}
	svc := &Service{
		mappings:    staticMappingRepo{mapping: mapping},
		connections: staticConnectionProvider{conn: conn, adapter: adapter},
		products:    staticProductRepo{},
		skus:        skus,
		skuMappings: &recordingSKUMappingRepo{},
	}

	if err := svc.SyncProduct(1); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if len(skus.created) != 1 {
		t.Fatalf("expected the new SKU to be created, got %d", len(skus.created))
	}
	// 成本 10 × (1 + 50%) = 15
	if got := skus.created[0].PriceAmount.String(); got != "15.00" {
		t.Fatalf("new SKU price = %s, want 15.00 (parent category markup 50%%)", got)
	}
}

// parentCategoryAdapter 返回「分类 2 的父分类是 1」的分类树，商品接口正常返回。
type parentCategoryAdapter struct {
	upstream.Adapter
	product *upstream.UpstreamProduct
}

func (a *parentCategoryAdapter) ListCategories(_ context.Context) (*upstream.CategoryListResult, error) {
	return &upstream.CategoryListResult{
		Supported: true,
		Categories: []upstream.UpstreamCategory{
			{ID: 1, Name: jsonmap.JSON{"zh-CN": "游戏"}},
			{ID: 2, ParentID: 1, Name: jsonmap.JSON{"zh-CN": "子类"}},
		},
	}, nil
}

func (a *parentCategoryAdapter) GetProduct(_ context.Context, _ uint) (*upstream.UpstreamProduct, error) {
	return a.product, nil
}

// creatingSKURepo 记录被创建的新 SKU。
type creatingSKURepo struct {
	mappingcontract.SKURepository
	created []*productdomain.ProductSKU
}

func (r *creatingSKURepo) Create(item *productdomain.ProductSKU) error {
	copied := *item
	copied.ID = uint(len(r.created) + 100)
	r.created = append(r.created, &copied)
	return nil
}

func (r *creatingSKURepo) GetByID(_ uint) (*productdomain.ProductSKU, error) {
	return nil, nil
}

// ListByProduct 供 recalcProductPrice 汇总最低成本价（即使关闭自动同步价格，成本价也要算）。
func (r *creatingSKURepo) ListByProduct(productID uint, onlyActive bool) ([]productdomain.ProductSKU, error) {
	var result []productdomain.ProductSKU
	for _, item := range r.created {
		if item.ProductID == productID && (!onlyActive || item.IsActive) {
			result = append(result, *item)
		}
	}
	return result, nil
}

// recordingSKUMappingRepo 接受新映射写入，不关心内容。
type recordingSKUMappingRepo struct {
	mappingcontract.SKUMappingRepository
}

func (r *recordingSKUMappingRepo) ListByProductMapping(_ uint) ([]mappingdomain.SKUMapping, error) {
	return nil, nil
}

func (r *recordingSKUMappingRepo) Create(_ *mappingdomain.SKUMapping) error {
	return nil
}

// staticProductRepo 返回一个固定的本地商品，供 SyncProduct 走完商品同步阶段。
type staticProductRepo struct {
	mappingcontract.ProductRepository
}

func (staticProductRepo) GetByID(_ string) (*productdomain.Product, error) {
	return &productdomain.Product{ID: 10}, nil
}

func (staticProductRepo) Update(_ *productdomain.Product) error {
	return nil
}

// recordingMappingRepo 把映射状态更新记录下来。
type recordingMappingRepo struct {
	mappingcontract.MappingRepository
	mapping *mappingdomain.Mapping
}

func (r *recordingMappingRepo) GetByID(_ uint) (*mappingdomain.Mapping, error) {
	return r.mapping, nil
}

func (r *recordingMappingRepo) Update(item *mappingdomain.Mapping) error {
	r.mapping = item
	return nil
}

// unsupportedCategoryAdapter 模拟分类接口 404：适配器返回 Supported=false 的空列表而不是错误。
type unsupportedCategoryAdapter struct{ upstream.Adapter }

func (unsupportedCategoryAdapter) ListCategories(_ context.Context) (*upstream.CategoryListResult, error) {
	return &upstream.CategoryListResult{Supported: false}, nil
}

// 配了分类加价时，「上游不支持分类」不能当成一棵空树，否则子分类商品会按默认加价改价。
func TestUpstreamCategoryParentsRejectsUnsupportedUpstream(t *testing.T) {
	conn := &siteconnectiondomain.Connection{ID: 1, CategoryMarkups: jsonmap.JSON{"1": float64(50)}}
	if _, ok := (&Service{}).upstreamCategoryParents(context.Background(), unsupportedCategoryAdapter{}, conn); ok {
		t.Fatal("expected ok=false when upstream does not support categories")
	}
}
