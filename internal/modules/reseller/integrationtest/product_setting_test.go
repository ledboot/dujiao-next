package integrationtest

import (
	"fmt"
	resellermodule "github.com/dujiao-next/internal/modules/reseller/application"
	"strings"
	"testing"
	"time"

	resellergormstore "github.com/dujiao-next/internal/modules/reseller/infrastructure/gormstore"

	resellerdomain "github.com/dujiao-next/internal/modules/reseller/domain"

	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"

	categorydomain "github.com/dujiao-next/internal/modules/catalog/category/domain"

	productgormstore "github.com/dujiao-next/internal/modules/catalog/product/store/gormstore"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func openResellerProductSettingServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:reseller_product_setting_service_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&userdomain.User{},
		&categorydomain.Category{},
		&productdomain.Product{},
		&productdomain.ProductSKU{},
		&resellerdomain.Profile{},
		&resellerdomain.ProductSetting{},
	); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return db
}

func seedResellerProductSettingServiceData(t *testing.T, db *gorm.DB) (userdomain.User, resellerdomain.Profile, productdomain.Product, []productdomain.ProductSKU) {
	t.Helper()
	user := userdomain.User{Email: fmt.Sprintf("setting-service-%d@example.test", time.Now().UnixNano()), PasswordHash: "hash", Status: constants.UserStatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	profile := resellerdomain.Profile{
		UserID:               user.ID,
		Status:               resellerdomain.ProfileStatusActive,
		DefaultMarkupPercent: money.FromDecimal(decimal.RequireFromString("10.00")),
		MaxMarkupPercent:     money.FromDecimal(decimal.RequireFromString("40.00")),
		SettlementStatus:     resellerdomain.SettlementStatusNormal,
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("create profile failed: %v", err)
	}
	category := categorydomain.Category{Slug: "service-category", NameJSON: jsonmap.JSON{"zh-CN": "分类"}, IsActive: true}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("create category failed: %v", err)
	}
	product := productdomain.Product{
		CategoryID:      category.ID,
		Slug:            "service-product",
		TitleJSON:       jsonmap.JSON{"zh-CN": "服务商品", "zh-TW": "服務商品", "en-US": "Service Product"},
		PriceAmount:     money.FromDecimal(decimal.RequireFromString("100.00")),
		CostPriceAmount: money.FromDecimal(decimal.RequireFromString("80.00")),
		IsActive:        true,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatalf("create product failed: %v", err)
	}
	skus := []productdomain.ProductSKU{
		{ProductID: product.ID, SKUCode: "A", PriceAmount: money.FromDecimal(decimal.RequireFromString("100.00")), CostPriceAmount: money.FromDecimal(decimal.RequireFromString("80.00")), IsActive: true},
		{ProductID: product.ID, SKUCode: "B", PriceAmount: money.FromDecimal(decimal.RequireFromString("200.00")), CostPriceAmount: money.FromDecimal(decimal.RequireFromString("120.00")), IsActive: true},
	}
	if err := db.Create(&skus).Error; err != nil {
		t.Fatalf("create skus failed: %v", err)
	}
	return user, profile, product, skus
}

func newResellerProductSettingServiceForTest(db *gorm.DB) *ResellerProductSettingService {
	return NewResellerProductSettingService(
		resellergormstore.New(db),
		resellergormstore.New(db),
		productgormstore.NewProductStore(db),
	)
}

func TestResellerProductSettingServiceUserSaveValidatesAndReturnsEffectivePrices(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	user, _, product, skus := seedResellerProductSettingServiceData(t, db)
	svc := newResellerProductSettingServiceForTest(db)

	detail, err := svc.SaveUserProductSettings(user.ID, product.ID, ResellerProductSettingSaveInput{
		Settings: []ResellerProductSettingInput{
			{SKUID: 0, IsListed: true, PricingMode: resellerdomain.PricingModeMarkupPercent, MarkupPercent: decimal.RequireFromString("20.00")},
			{SKUID: skus[0].ID, IsListed: true, PricingMode: resellerdomain.PricingModeFixedPrice, FixedPriceAmount: decimal.RequireFromString("130.00")},
		},
	})
	if err != nil {
		t.Fatalf("save user settings failed: %v", err)
	}
	if detail == nil || detail.Product.ID != product.ID || len(detail.Settings) != 2 {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	effective := detail.EffectiveBySKUID[skus[0].ID]
	if effective.StringFixed(2) != "130.00" {
		t.Fatalf("sku override effective price want 130.00 got %s", effective.StringFixed(2))
	}
	fallback := detail.EffectiveBySKUID[skus[1].ID]
	if fallback.StringFixed(2) != "240.00" {
		t.Fatalf("product markup effective price want 240.00 got %s", fallback.StringFixed(2))
	}
}

func TestResellerProductSettingServicePersistsFirstHiddenSKUSetting(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	user, profile, product, skus := seedResellerProductSettingServiceData(t, db)
	svc := newResellerProductSettingServiceForTest(db)

	detail, err := svc.SaveUserProductSettings(user.ID, product.ID, ResellerProductSettingSaveInput{
		Settings: []ResellerProductSettingInput{
			{SKUID: skus[0].ID, IsListed: false, PricingMode: resellerdomain.PricingModeInherit},
		},
	})
	if err != nil {
		t.Fatalf("save hidden sku setting failed: %v", err)
	}
	if detail == nil {
		t.Fatal("expected detail after hidden save")
	}

	var row resellerdomain.ProductSetting
	if err := db.Where("reseller_id = ? AND product_id = ? AND sku_id = ?", profile.ID, product.ID, skus[0].ID).
		First(&row).Error; err != nil {
		t.Fatalf("fetch hidden sku setting failed: %v", err)
	}
	if row.IsListed {
		t.Fatalf("expected first hidden sku save to persist is_listed=false, got true: %+v", row)
	}
}

func TestResellerProductSettingServiceRejectsPriceBelowBase(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	user, _, product, skus := seedResellerProductSettingServiceData(t, db)
	svc := newResellerProductSettingServiceForTest(db)

	_, err := svc.SaveUserProductSettings(user.ID, product.ID, ResellerProductSettingSaveInput{
		Settings: []ResellerProductSettingInput{
			{SKUID: skus[0].ID, IsListed: true, PricingMode: resellerdomain.PricingModeFixedPrice, FixedPriceAmount: decimal.RequireFromString("99.99")},
		},
	})
	if err != ErrResellerPriceBelowBase {
		t.Fatalf("expected ErrResellerPriceBelowBase, got %v", err)
	}
}

func TestResellerProductSettingServiceRejectsMarkupExceeded(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	user, _, product, skus := seedResellerProductSettingServiceData(t, db)
	svc := newResellerProductSettingServiceForTest(db)

	_, err := svc.SaveUserProductSettings(user.ID, product.ID, ResellerProductSettingSaveInput{
		Settings: []ResellerProductSettingInput{
			{SKUID: skus[0].ID, IsListed: true, PricingMode: resellerdomain.PricingModeFixedPrice, FixedPriceAmount: decimal.RequireFromString("150.00")},
		},
	})
	if err != ErrResellerMarkupExceeded {
		t.Fatalf("expected ErrResellerMarkupExceeded, got %v", err)
	}
}

func TestResellerProductSettingServiceRejectsBatchWithoutPartialWrites(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	user, profile, product, skus := seedResellerProductSettingServiceData(t, db)
	svc := newResellerProductSettingServiceForTest(db)

	_, err := svc.SaveUserProductSettings(user.ID, product.ID, ResellerProductSettingSaveInput{
		Settings: []ResellerProductSettingInput{
			{SKUID: skus[0].ID, IsListed: true, PricingMode: resellerdomain.PricingModeFixedPrice, FixedPriceAmount: decimal.RequireFromString("130.00")},
			{SKUID: skus[1].ID, IsListed: true, PricingMode: resellerdomain.PricingModeFixedPrice, FixedPriceAmount: decimal.RequireFromString("99.99")},
		},
	})
	if err != ErrResellerPriceBelowBase {
		t.Fatalf("expected ErrResellerPriceBelowBase, got %v", err)
	}

	var count int64
	if err := db.Model(&resellerdomain.ProductSetting{}).
		Where("reseller_id = ? AND product_id = ?", profile.ID, product.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count settings failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no partial writes after rejected batch, got %d settings", count)
	}
}

func findPreviewItem(items []ResellerProductSettingPreviewItem, skuID uint) *ResellerProductSettingPreviewItem {
	for i := range items {
		if items[i].SKUID == skuID {
			return &items[i]
		}
	}
	return nil
}

func TestResellerProductSettingServicePreviewMatchesSaveSemantics(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	user, profile, product, skus := seedResellerProductSettingServiceData(t, db)
	svc := newResellerProductSettingServiceForTest(db)

	items, err := svc.PreviewUserProductSettings(user.ID, product.ID, ResellerProductSettingSaveInput{
		Settings: []ResellerProductSettingInput{
			{SKUID: 0, IsListed: true, PricingMode: resellerdomain.PricingModeMarkupPercent, MarkupPercent: decimal.RequireFromString("20.00")},
			{SKUID: skus[0].ID, IsListed: true, PricingMode: resellerdomain.PricingModeFixedPrice, FixedPriceAmount: decimal.RequireFromString("130.00")},
			{SKUID: skus[1].ID, IsListed: true, PricingMode: resellerdomain.PricingModeInherit},
		},
	})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	skuA := findPreviewItem(items, skus[0].ID)
	if skuA == nil || !skuA.Valid || skuA.EffectivePrice.StringFixed(2) != "130.00" {
		t.Fatalf("sku A preview want valid 130.00, got %+v", skuA)
	}
	skuB := findPreviewItem(items, skus[1].ID)
	if skuB == nil || !skuB.Valid || skuB.EffectivePrice.StringFixed(2) != "240.00" {
		t.Fatalf("sku B inherit preview want valid 240.00, got %+v", skuB)
	}

	// 预览不得落库。
	var count int64
	if err := db.Model(&resellerdomain.ProductSetting{}).Where("reseller_id = ?", profile.ID).Count(&count).Error; err != nil {
		t.Fatalf("count settings failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("preview must not persist settings, found %d rows", count)
	}
}

func TestResellerProductSettingServicePreviewFlagsInvalidRules(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	user, _, product, skus := seedResellerProductSettingServiceData(t, db)
	svc := newResellerProductSettingServiceForTest(db)

	// 低于基准价 → price_invalid；超出封顶加价 → markup_exceeded。
	items, err := svc.PreviewUserProductSettings(user.ID, product.ID, ResellerProductSettingSaveInput{
		Settings: []ResellerProductSettingInput{
			{SKUID: skus[0].ID, IsListed: true, PricingMode: resellerdomain.PricingModeFixedPrice, FixedPriceAmount: decimal.RequireFromString("99.99")},
			{SKUID: skus[1].ID, IsListed: true, PricingMode: resellerdomain.PricingModeFixedPrice, FixedPriceAmount: decimal.RequireFromString("300.00")},
		},
	})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	skuA := findPreviewItem(items, skus[0].ID)
	if skuA == nil || skuA.Valid || skuA.ErrorCode != "price_invalid" {
		t.Fatalf("sku A want invalid price_invalid, got %+v", skuA)
	}
	skuB := findPreviewItem(items, skus[1].ID)
	if skuB == nil || skuB.Valid || skuB.ErrorCode != "markup_exceeded" {
		t.Fatalf("sku B want invalid markup_exceeded, got %+v", skuB)
	}
}

func TestResellerProductSettingServiceRequiresActiveProfile(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	user, profile, product, _ := seedResellerProductSettingServiceData(t, db)
	if err := db.Model(&resellerdomain.Profile{}).Where("id = ?", profile.ID).Update("status", resellerdomain.ProfileStatusPendingReview).Error; err != nil {
		t.Fatalf("update profile failed: %v", err)
	}
	svc := newResellerProductSettingServiceForTest(db)
	_, _, err := svc.ListUserProductSettings(user.ID, ResellerProductSettingUserListInput{Page: 1, PageSize: 20})
	if err != ErrResellerProfileInactive {
		t.Fatalf("expected inactive profile, got %v product=%d", err, product.ID)
	}
}

func TestResellerSupplyPriceAdminOwnershipAndFallback(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	user, profile, product, skus := seedResellerProductSettingServiceData(t, db)
	svc := newResellerProductSettingServiceForTest(db)
	supply := decimal.NewFromInt(90)
	payload := ResellerProductSettingSaveInput{Settings: []ResellerProductSettingInput{{SKUID: skus[0].ID, IsListed: true, PricingMode: resellerdomain.PricingModeFixedPrice, FixedPriceAmount: decimal.NewFromInt(95), SupplyPriceAmount: &supply}}}
	preview, err := svc.PreviewAdminProductSettings(profile.ID, product.ID, payload)
	if err != nil {
		t.Fatal(err)
	}
	if item := findPreviewItem(preview, skus[0].ID); item == nil || !item.Valid || item.BasePrice.StringFixed(2) != "90.00" || item.EffectivePrice.StringFixed(2) != "95.00" {
		t.Fatalf("unexpected preview: %+v", item)
	}
	detail, err := svc.SaveAdminProductSettings(profile.ID, product.ID, payload)
	if err != nil {
		t.Fatal(err)
	}
	if detail.EffectiveBySKUID[skus[0].ID].StringFixed(2) != "95.00" || detail.Product.SKUs[0].PriceAmount.StringFixed(2) != "100.00" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	payload.Settings[0].SupplyPriceAmount = nil
	detail, err = svc.SaveAdminProductSettings(profile.ID, product.ID, payload)
	if err != nil || detail.Settings[0].SupplyPriceAmount.StringFixed(2) != "90.00" {
		t.Fatalf("omitted supply override was lost: %v", err)
	}
	// A reseller cannot replace the administrator-owned price, even via a forged request.
	forged := decimal.NewFromInt(1)
	payload.Settings[0].SupplyPriceAmount = &forged
	detail, err = svc.SaveUserProductSettings(user.ID, product.ID, payload)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Settings[0].SupplyPriceAmount.StringFixed(2) != "90.00" {
		t.Fatal("user changed supply price")
	}
	if err := svc.ResetUserProductSetting(user.ID, product.ID, skus[0].ID); err != nil {
		t.Fatal(err)
	}
	detail, err = svc.GetUserProductSetting(user.ID, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Settings[0].SupplyPriceAmount.StringFixed(2) != "90.00" || detail.EffectiveBySKUID[skus[0].ID].StringFixed(2) != "99.00" {
		t.Fatal("reset must retain supply price and restore default markup")
	}
	// Explicit zero clears the override; omitted values retain it.
	zero := decimal.Zero
	payload.Settings[0].SupplyPriceAmount = &zero
	payload.Settings[0].PricingMode = resellerdomain.PricingModeInherit
	detail, err = svc.SaveAdminProductSettings(profile.ID, product.ID, payload)
	if err != nil {
		t.Fatal(err)
	}
	if detail.EffectiveBySKUID[skus[0].ID].StringFixed(2) != "110.00" {
		t.Fatal("zero must restore retail base")
	}
	for _, invalid := range []string{"-1", "0.001", "79.99"} {
		value := decimal.RequireFromString(invalid)
		payload.Settings[0].SupplyPriceAmount = &value
		if _, err := svc.SaveAdminProductSettings(profile.ID, product.ID, payload); err == nil {
			t.Fatalf("accepted invalid supply price %s", invalid)
		}
	}
}

func TestResellerProductImagesPersistAndRemainIsolated(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	user, _, product, skus := seedResellerProductSettingServiceData(t, db)
	otherUser := userdomain.User{Email: "other-images@example.test", PasswordHash: "hash", Status: constants.UserStatusActive}
	if err := db.Create(&otherUser).Error; err != nil {
		t.Fatal(err)
	}
	otherProfile := resellerdomain.Profile{UserID: otherUser.ID, Status: resellerdomain.ProfileStatusActive}
	if err := db.Create(&otherProfile).Error; err != nil {
		t.Fatal(err)
	}
	svc := newResellerProductSettingServiceForTest(db)
	save := func(images []string) *resellermodule.ProductSettingDetail {
		t.Helper()
		detail, err := svc.SaveUserProductSettings(user.ID, product.ID, ResellerProductSettingSaveInput{Settings: []ResellerProductSettingInput{{IsListed: true, PricingMode: "inherit", Images: images}}})
		if err != nil {
			t.Fatal(err)
		}
		return detail
	}
	images := []string{"/uploads/reseller-cover.png", "https://example.com/detail.png"}
	detail := save(images)
	if len(detail.Settings[0].Images) != 2 {
		t.Fatalf("images not persisted: %+v", detail.Settings)
	}
	detail = save(nil)
	if len(detail.Settings[0].Images) != 2 {
		t.Fatal("omitted images erased override")
	}
	other, err := svc.GetUserProductSetting(otherUser.ID, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Settings) != 0 {
		t.Fatal("images leaked to another reseller")
	}
	var main productdomain.Product
	if err := db.First(&main, product.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(main.Images) != len(product.Images) {
		t.Fatal("main product images changed")
	}
	for _, input := range []ResellerProductSettingInput{
		{IsListed: true, Images: []string{"javascript:alert(1)"}},
		{IsListed: true, SKUID: skus[0].ID, Images: images},
		{IsListed: true, Images: make([]string, 11)},
	} {
		if _, err := svc.SaveUserProductSettings(user.ID, product.ID, ResellerProductSettingSaveInput{Settings: []ResellerProductSettingInput{input}}); err == nil {
			t.Fatal("invalid image input accepted")
		}
	}
	detail = save([]string{})
	if len(detail.Settings[0].Images) != 0 {
		t.Fatal("empty images did not restore inheritance")
	}
	save(images)
	if err := svc.ResetUserProductSetting(user.ID, product.ID, 0); err != nil {
		t.Fatal(err)
	}
	detail, err = svc.GetUserProductSetting(user.ID, product.ID)
	if err != nil || len(detail.Settings) != 0 {
		t.Fatalf("reset failed: %+v %v", detail, err)
	}
	detail = save(nil)
	if len(detail.Settings[0].Images) != 0 {
		t.Fatal("reset images returned after saving price rule")
	}
}

func TestResellerProductTitleOverrides(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	user, _, product, skus := seedResellerProductSettingServiceData(t, db)
	svc := newResellerProductSettingServiceForTest(db)
	save := func(title map[string]string) *resellermodule.ProductSettingDetail {
		t.Helper()
		detail, err := svc.SaveUserProductSettings(user.ID, product.ID, ResellerProductSettingSaveInput{Settings: []ResellerProductSettingInput{{IsListed: true, PricingMode: "inherit", Title: title}}})
		if err != nil {
			t.Fatal(err)
		}
		return detail
	}
	detail := save(map[string]string{"zh-CN": " 自定义商品 ", "en-US": ""})
	if detail.Settings[0].TitleJSON["zh-CN"] != "自定义商品" {
		t.Fatalf("title not saved: %+v", detail.Settings)
	}
	if _, exists := detail.Settings[0].TitleJSON["en-US"]; exists {
		t.Fatal("blank translation must inherit")
	}
	detail = save(nil)
	if detail.Settings[0].TitleJSON["zh-CN"] != "自定义商品" {
		t.Fatal("omitted title erased override")
	}
	var main productdomain.Product
	if err := db.First(&main, product.ID).Error; err != nil {
		t.Fatal(err)
	}
	if main.TitleJSON["zh-CN"] != product.TitleJSON["zh-CN"] {
		t.Fatal("main title changed")
	}
	for _, input := range []ResellerProductSettingInput{
		{IsListed: true, Title: map[string]string{"fr": "invalid"}},
		{IsListed: true, Title: map[string]string{"zh-CN": strings.Repeat("字", 201)}},
		{IsListed: true, SKUID: skus[0].ID, Title: map[string]string{"zh-CN": "invalid"}},
	} {
		if _, err := svc.SaveUserProductSettings(user.ID, product.ID, ResellerProductSettingSaveInput{Settings: []ResellerProductSettingInput{input}}); err == nil {
			t.Fatal("invalid title accepted")
		}
	}
	detail = save(map[string]string{})
	if len(detail.Settings[0].TitleJSON) != 0 {
		t.Fatal("empty title did not restore inheritance")
	}
	save(map[string]string{"zh-CN": "override"})
	if err := svc.ResetUserProductSetting(user.ID, product.ID, 0); err != nil {
		t.Fatal(err)
	}
	detail = save(nil)
	if len(detail.Settings[0].TitleJSON) != 0 {
		t.Fatal("reset title returned after saving rule")
	}
}
