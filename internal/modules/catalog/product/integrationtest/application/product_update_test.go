package integrationtest

import (
	"errors"
	"strconv"
	"testing"

	productwrite "github.com/dujiao-next/internal/modules/catalog/product/application/write"
	productcontract "github.com/dujiao-next/internal/modules/catalog/product/contract"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"

	categorydomain "github.com/dujiao-next/internal/modules/catalog/category/domain"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
)

func TestProductServiceUpdateKeepsMappedProductFulfillmentUpstream(t *testing.T) {
	svc, db := newProductServiceForTest(t)

	category := categorydomain.Category{
		Slug:     "mapped-category",
		NameJSON: jsonmap.JSON{"zh-CN": "mapped-category"},
	}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("create category failed: %v", err)
	}

	product := productdomain.Product{
		CategoryID:       category.ID,
		Slug:             "mapped-product",
		TitleJSON:        jsonmap.JSON{"zh-CN": "mapped-product"},
		PriceAmount:      money.FromDecimal(decimal.NewFromInt(10)),
		PurchaseType:     constants.ProductPurchaseMember,
		FulfillmentType:  constants.FulfillmentTypeUpstream,
		ManualStockTotal: 0,
		IsMapped:         true,
		IsActive:         true,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create mapped product failed: %v", err)
	}

	sku := productdomain.ProductSKU{
		ProductID:      product.ID,
		SKUCode:        productdomain.DefaultSKUCode,
		SpecValuesJSON: jsonmap.JSON{},
		PriceAmount:    money.FromDecimal(decimal.NewFromInt(10)),
		IsActive:       true,
	}
	if err := db.Create(&sku).Error; err != nil {
		t.Fatalf("create mapped product sku failed: %v", err)
	}

	updated, err := svc.Write.Update(strconv.FormatUint(uint64(product.ID), 10), productwrite.CreateProductInput{
		CategoryID:      category.ID,
		Slug:            "mapped-product-updated",
		TitleJSON:       map[string]interface{}{"zh-CN": "mapped-product-updated"},
		PriceAmount:     decimal.NewFromInt(20),
		PurchaseType:    constants.ProductPurchaseMember,
		FulfillmentType: constants.FulfillmentTypeAuto,
		ManualStockTotal: func() *int {
			value := 0
			return &value
		}(),
		IsActive: func() *bool {
			value := true
			return &value
		}(),
	})
	if err != nil {
		t.Fatalf("update mapped product failed: %v", err)
	}
	if updated.FulfillmentType != constants.FulfillmentTypeUpstream {
		t.Fatalf("expected mapped product fulfillment type to remain upstream, got %s", updated.FulfillmentType)
	}

	reloaded, err := svc.Read.GetAdminByID(strconv.FormatUint(uint64(product.ID), 10))
	if err != nil {
		t.Fatalf("reload mapped product failed: %v", err)
	}
	if reloaded.FulfillmentType != constants.FulfillmentTypeUpstream {
		t.Fatalf("expected persisted fulfillment type upstream, got %s", reloaded.FulfillmentType)
	}
}

func TestProductServiceUpdateFiltersUnavailablePaymentChannels(t *testing.T) {
	svc, db := newProductServiceForTest(t)

	category := categorydomain.Category{
		Slug:     "payment-channel-update-category",
		NameJSON: jsonmap.JSON{"zh-CN": "payment-channel-update-category"},
	}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("create category failed: %v", err)
	}

	deletedChannel := createProductTestPaymentChannel(t, db, "Deleted", true, true)
	product := productdomain.Product{
		CategoryID:        category.ID,
		Slug:              "payment-channel-update",
		TitleJSON:         jsonmap.JSON{"zh-CN": "payment-channel-update"},
		PriceAmount:       money.FromDecimal(decimal.NewFromInt(10)),
		PurchaseType:      constants.ProductPurchaseMember,
		FulfillmentType:   constants.FulfillmentTypeAuto,
		PaymentChannelIDs: productdomain.EncodePaymentChannelIDs([]uint{deletedChannel.ID}),
		IsActive:          true,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product failed: %v", err)
	}

	updated, err := svc.Write.Update(strconv.FormatUint(uint64(product.ID), 10), productwrite.CreateProductInput{
		CategoryID:        category.ID,
		Slug:              product.Slug,
		TitleJSON:         map[string]interface{}{"zh-CN": "payment-channel-update"},
		PriceAmount:       decimal.NewFromInt(10),
		PurchaseType:      constants.ProductPurchaseMember,
		FulfillmentType:   constants.FulfillmentTypeAuto,
		PaymentChannelIDs: []uint{deletedChannel.ID},
		IsActive: func() *bool {
			value := true
			return &value
		}(),
	})
	if err != nil {
		t.Fatalf("update product failed: %v", err)
	}
	if got := productdomain.DecodePaymentChannelIDs(updated.PaymentChannelIDs); len(got) != 0 {
		t.Fatalf("expected stale-only payment channels to be cleared, got %v", got)
	}
}

func TestProductServiceUpdateRejectsInvalidPurchaseLimits(t *testing.T) {
	svc, db := newProductServiceForTest(t)

	cat := categorydomain.Category{Slug: "test-purchase-limit-update", NameJSON: jsonmap.JSON{"zh-CN": "test"}}
	if err := db.Create(&cat).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}

	intPtr := func(v int) *int { return &v }

	created, err := svc.Write.Create(productwrite.CreateProductInput{
		CategoryID:          cat.ID,
		Slug:                "valid-limit-product",
		TitleJSON:           map[string]interface{}{"zh-CN": "valid"},
		PriceAmount:         decimal.NewFromInt(10),
		PurchaseType:        constants.ProductPurchaseMember,
		FulfillmentType:     constants.FulfillmentTypeManual,
		ManualStockTotal:    intPtr(1),
		MinPurchaseQuantity: intPtr(2),
		MaxPurchaseQuantity: intPtr(5),
	})
	if err != nil {
		t.Fatalf("create product failed: %v", err)
	}

	_, err = svc.Write.Update(strconv.FormatUint(uint64(created.ID), 10), productwrite.CreateProductInput{
		CategoryID:          cat.ID,
		Slug:                "valid-limit-product",
		TitleJSON:           map[string]interface{}{"zh-CN": "valid"},
		PriceAmount:         decimal.NewFromInt(10),
		PurchaseType:        constants.ProductPurchaseMember,
		FulfillmentType:     constants.FulfillmentTypeManual,
		ManualStockTotal:    intPtr(1),
		MaxPurchaseQuantity: intPtr(1), // 已存在 min=2，新设 max=1 应触发校验
	})
	if !errors.Is(err, productcontract.ErrProductPurchaseLimitInvalid) {
		t.Fatalf("expected productcontract.ErrProductPurchaseLimitInvalid on update, got %v", err)
	}
}

func TestDefaultSupplyPricesPersistAndPreserveOmittedValues(t *testing.T) {
	svc, db := newProductServiceForTest(t)
	category := categorydomain.Category{Slug: "supply-default", NameJSON: jsonmap.JSON{"zh-CN": "supply"}}
	if err := db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	amount := func(v int64) *money.Amount { m := money.FromDecimal(decimal.NewFromInt(v)); return &m }
	input := productwrite.CreateProductInput{CategoryID: category.ID, Slug: "default-supply", TitleJSON: map[string]interface{}{"zh-CN": "supply"}, PriceAmount: decimal.NewFromInt(120), CostPriceAmount: decimal.NewFromInt(100), PurchaseType: constants.ProductPurchaseMember, FulfillmentType: constants.FulfillmentTypeManual, DefaultSupplyPriceAmount: amount(115), SKUs: []productwrite.ProductSKUInput{{SKUCode: "A", PriceAmount: decimal.NewFromInt(120), CostPriceAmount: decimal.NewFromInt(100), DefaultSupplyPriceAmount: amount(112)}}}
	product, err := svc.Write.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	var skus []productdomain.ProductSKU
	if err := db.Where("product_id = ?", product.ID).Find(&skus).Error; err != nil || len(skus) != 1 {
		t.Fatalf("skus: %v %v", skus, err)
	}
	if product.DefaultSupplyPriceAmount.String() != "115.00" || skus[0].DefaultSupplyPriceAmount.String() != "112.00" {
		t.Fatal("defaults not persisted")
	}
	input.SKUs[0].ID = skus[0].ID
	input.DefaultSupplyPriceAmount = nil
	input.SKUs[0].DefaultSupplyPriceAmount = nil
	product, err = svc.Write.Update(strconv.Itoa(int(product.ID)), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.First(&skus[0], skus[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if product.DefaultSupplyPriceAmount.String() != "115.00" || skus[0].DefaultSupplyPriceAmount.String() != "112.00" {
		t.Fatal("omitted values overwritten")
	}
	input.DefaultSupplyPriceAmount = amount(0)
	input.SKUs[0].DefaultSupplyPriceAmount = amount(0)
	product, err = svc.Write.Update(strconv.Itoa(int(product.ID)), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.First(&skus[0], skus[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if !product.DefaultSupplyPriceAmount.IsZero() || !skus[0].DefaultSupplyPriceAmount.IsZero() {
		t.Fatal("reset failed")
	}
	input.DefaultSupplyPriceAmount = amount(99)
	if _, err = svc.Write.Update(strconv.Itoa(int(product.ID)), input); !errors.Is(err, productcontract.ErrProductPriceInvalid) {
		t.Fatalf("below-cost fallback accepted: %v", err)
	}
	input.DefaultSupplyPriceAmount = amount(115)
	input.SKUs[0].DefaultSupplyPriceAmount = amount(-1)
	if _, err = svc.Write.Update(strconv.Itoa(int(product.ID)), input); !errors.Is(err, productcontract.ErrProductPriceInvalid) {
		t.Fatalf("negative SKU supply accepted: %v", err)
	}
}
