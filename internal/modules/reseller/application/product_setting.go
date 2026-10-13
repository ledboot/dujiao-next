package application

import (
	"errors"
	"strconv"
	"strings"

	resellercontract "github.com/dujiao-next/internal/modules/reseller/contract"

	resellerdomain "github.com/dujiao-next/internal/modules/reseller/domain"

	productcontract "github.com/dujiao-next/internal/modules/catalog/product/contract"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"

	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
)

// ProductSettingService 分销商品改价/上架配置用例。
type ProductSettingService struct {
	store       resellercontract.ProductSettingStore
	productRepo productcontract.Repository
}

func NewProductSettingService(store resellercontract.ProductSettingStore, productRepo productcontract.Repository) *ProductSettingService {
	return &ProductSettingService{store: store, productRepo: productRepo}
}

type ProductSettingInput struct {
	SupplyPriceAmount *decimal.Decimal
	SKUID             uint
	IsListed          bool
	PricingMode       string
	MarkupPercent     decimal.Decimal
	FixedMarkupAmount decimal.Decimal
	FixedPriceAmount  decimal.Decimal
	SortOrder         int
}

type ProductSettingSaveInput struct {
	admin    bool
	Settings []ProductSettingInput
}

type ProductSettingUserListInput struct {
	Page       int
	PageSize   int
	Keyword    string
	CategoryID uint
	Configured string
	Listed     string
}

type ProductSettingAdminListInput struct {
	Page        int
	PageSize    int
	ResellerID  uint
	UserID      uint
	ProductID   uint
	Keyword     string
	PricingMode string
	Configured  string
	Listed      string
}

type ProductSettingDetail struct {
	Profile          *resellerdomain.Profile
	Product          productdomain.Product
	Settings         []resellerdomain.ProductSetting
	EffectiveBySKUID map[uint]decimal.Decimal
	RuleBySKUID      map[uint]string
}

type ProductSettingListRow struct {
	Profile          *resellerdomain.Profile
	Product          productdomain.Product
	Settings         []resellerdomain.ProductSetting
	EffectiveBySKUID map[uint]decimal.Decimal
	RuleBySKUID      map[uint]string
}

// ProductSettingPreviewItem 表示某个商品级（SKUID=0）或 SKU 级规则在「拟用配置」下的预览结果。
// 复用与保存/下单完全一致的 ResolveUnitAmount + ValidateUnitAmount，确保预览价与实际成交价零分歧。
type ProductSettingPreviewItem struct {
	SKUID          uint
	IsListed       bool
	BasePrice      decimal.Decimal
	EffectivePrice decimal.Decimal
	Valid          bool
	ErrorCode      string
}

func (s *ProductSettingService) ListUserProductSettings(userID uint, input ProductSettingUserListInput) ([]ProductSettingListRow, int64, error) {
	profile, err := s.requireActiveProfileByUser(userID)
	if err != nil {
		return nil, 0, err
	}
	rows, total, err := s.store.ListProductsWithSettings(resellercontract.ProductSettingListFilter{
		Page:       input.Page,
		PageSize:   input.PageSize,
		ResellerID: profile.ID,
		CategoryID: input.CategoryID,
		Keyword:    input.Keyword,
		Configured: input.Configured,
		Listed:     input.Listed,
		OnlyActive: true,
	})
	if err != nil {
		return nil, 0, err
	}
	return s.decorateRows(profile, rows), total, nil
}

func (s *ProductSettingService) GetUserProductSetting(userID, productID uint) (*ProductSettingDetail, error) {
	profile, err := s.requireActiveProfileByUser(userID)
	if err != nil {
		return nil, err
	}
	return s.getDetail(profile, productID)
}

// PreviewUserProductSettings 在不落库的前提下，按用户拟用的定价规则计算各 SKU 的预计生效价与校验结果。
func (s *ProductSettingService) PreviewUserProductSettings(userID, productID uint, input ProductSettingSaveInput) ([]ProductSettingPreviewItem, error) {
	profile, err := s.requireActiveProfileByUser(userID)
	if err != nil {
		return nil, err
	}
	return s.previewSettings(profile, productID, input)
}

func (s *ProductSettingService) SaveUserProductSettings(userID, productID uint, input ProductSettingSaveInput) (*ProductSettingDetail, error) {
	profile, err := s.requireActiveProfileByUser(userID)
	if err != nil {
		return nil, err
	}
	if err := s.saveSettings(profile, productID, input); err != nil {
		return nil, err
	}
	return s.getDetail(profile, productID)
}

func (s *ProductSettingService) ResetUserProductSetting(userID, productID, skuID uint) error {
	profile, err := s.requireActiveProfileByUser(userID)
	if err != nil {
		return err
	}
	return s.store.DeleteSetting(profile.ID, productID, skuID)
}

func (s *ProductSettingService) ListAdminSettings(input ProductSettingAdminListInput) ([]resellerdomain.ProductSetting, int64, error) {
	return s.store.ListAdminSettings(resellercontract.ProductSettingAdminListFilter(input))
}

func (s *ProductSettingService) SummarizeAdminSettings(resellerID uint) (resellercontract.ProductSettingSummary, error) {
	if s == nil || s.store == nil || resellerID == 0 {
		return resellercontract.ProductSettingSummary{}, nil
	}
	return s.store.SummarizeByResellerID(resellerID)
}

func (s *ProductSettingService) GetAdminProductSetting(resellerID, productID uint) (*ProductSettingDetail, error) {
	profile, err := s.requireActiveProfileByID(resellerID)
	if err != nil {
		return nil, err
	}
	return s.getDetail(profile, productID)
}

// PreviewAdminProductSettings 在不落库的前提下，按管理员拟用的定价规则计算各 SKU 的预计生效价与校验结果。
func (s *ProductSettingService) PreviewAdminProductSettings(resellerID, productID uint, input ProductSettingSaveInput) ([]ProductSettingPreviewItem, error) {
	input.admin = true
	profile, err := s.requireActiveProfileByID(resellerID)
	if err != nil {
		return nil, err
	}
	return s.previewSettings(profile, productID, input)
}

func (s *ProductSettingService) SaveAdminProductSettings(resellerID, productID uint, input ProductSettingSaveInput) (*ProductSettingDetail, error) {
	input.admin = true
	profile, err := s.requireActiveProfileByID(resellerID)
	if err != nil {
		return nil, err
	}
	if err := s.saveSettings(profile, productID, input); err != nil {
		return nil, err
	}
	return s.getDetail(profile, productID)
}

func (s *ProductSettingService) ResetAdminProductSetting(resellerID, productID, skuID uint) error {
	profile, err := s.requireActiveProfileByID(resellerID)
	if err != nil {
		return err
	}
	return s.store.DeleteSetting(profile.ID, productID, skuID)
}

func (s *ProductSettingService) requireActiveProfileByUser(userID uint) (*resellerdomain.Profile, error) {
	if s == nil || s.store == nil || s.productRepo == nil || userID == 0 {
		return nil, productcontract.ErrNotFound
	}
	profile, err := s.store.GetProfileByUserID(userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, resellercontract.ErrNotOpened
	}
	if profile.Status != resellerdomain.ProfileStatusActive {
		return nil, resellercontract.ErrProfileInactive
	}
	return profile, nil
}

func (s *ProductSettingService) requireActiveProfileByID(resellerID uint) (*resellerdomain.Profile, error) {
	if s == nil || s.store == nil || s.productRepo == nil || resellerID == 0 {
		return nil, productcontract.ErrNotFound
	}
	profile, err := s.store.GetProfileByID(resellerID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, productcontract.ErrNotFound
	}
	if profile.Status != resellerdomain.ProfileStatusActive {
		return nil, resellercontract.ErrProfileInactive
	}
	return profile, nil
}

func (s *ProductSettingService) getDetail(profile *resellerdomain.Profile, productID uint) (*ProductSettingDetail, error) {
	if profile == nil || productID == 0 {
		return nil, productcontract.ErrNotFound
	}
	row, err := s.store.GetProductWithSettings(profile.ID, productID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, productcontract.ErrNotFound
	}
	effective, rules, err := computeProductEffectivePrices(profile, row.Product, row.Settings)
	if err != nil {
		return nil, err
	}
	return &ProductSettingDetail{Profile: profile, Product: row.Product, Settings: row.Settings, EffectiveBySKUID: effective, RuleBySKUID: rules}, nil
}

func (s *ProductSettingService) decorateRows(profile *resellerdomain.Profile, rows []resellercontract.ProductSettingProductRow) []ProductSettingListRow {
	out := make([]ProductSettingListRow, 0, len(rows))
	for _, row := range rows {
		effective, rules, _ := computeProductEffectivePrices(profile, row.Product, row.Settings)
		out = append(out, ProductSettingListRow{Profile: profile, Product: row.Product, Settings: row.Settings, EffectiveBySKUID: effective, RuleBySKUID: rules})
	}
	return out
}

func (s *ProductSettingService) saveSettings(profile *resellerdomain.Profile, productID uint, input ProductSettingSaveInput) error {
	if profile == nil || productID == 0 {
		return productcontract.ErrNotFound
	}
	product, err := s.productRepo.GetAdminByID(strconv.FormatUint(uint64(productID), 10))
	if err != nil {
		return err
	}
	if product == nil {
		return productcontract.ErrNotFound
	}
	if err := validateSettingSKUs(*product, input); err != nil {
		return err
	}
	return s.store.WithinProductSettingTransaction(func(store resellercontract.ProductSettingStore) error {
		row, err := store.GetProductWithSettings(profile.ID, productID)
		if err != nil {
			return err
		}
		if row == nil {
			return productcontract.ErrNotFound
		}
		settings, err := mergeProductSettings(row.Settings, input)
		if err != nil {
			return err
		}
		if err := validateSupplySettings(profile, *product, settings); err != nil {
			return err
		}
		byProduct, bySKU := indexProductSettings(settings)
		for _, item := range input.Settings {
			setting := bySKU[productSettingKey{productID: productID, skuID: item.SKUID}]
			if item.SKUID == 0 {
				setting = byProduct[productID]
			}
			if setting == nil {
				return productcontract.ErrProductSKUInvalid
			}
			setting.ResellerID = profile.ID
			setting.UpdateSupplyPrice = input.admin && item.SupplyPriceAmount != nil
			if _, err := store.UpsertSetting(*setting); err != nil {
				return err
			}
		}
		return nil
	})
}

// mergeProductSettings retains omitted rules and protects the administrator-owned supply price.
func mergeProductSettings(existing []resellerdomain.ProductSetting, input ProductSettingSaveInput) ([]resellerdomain.ProductSetting, error) {
	rows := append([]resellerdomain.ProductSetting(nil), existing...)
	seen := map[uint]bool{}
	for _, item := range input.Settings {
		if seen[item.SKUID] {
			return nil, resellercontract.ErrPricingModeInvalid
		}
		seen[item.SKUID] = true
		setting, err := buildPreviewSetting(item)
		if err != nil {
			return nil, err
		}
		index := -1
		for i := range rows {
			if rows[i].SKUID == item.SKUID {
				index = i
				setting = copySupplySetting(setting, rows[i])
				break
			}
		}
		if input.admin && item.SupplyPriceAmount != nil {
			value := *item.SupplyPriceAmount
			if value.IsNegative() || (value.IsPositive() && value.Round(2).IsZero()) {
				return nil, resellercontract.ErrPriceBelowBase
			}
			setting.SupplyPriceAmount = money.FromDecimal(value.Round(2))
		}
		if index >= 0 {
			rows[index] = setting
		} else {
			rows = append(rows, setting)
		}
	}
	return rows, nil
}

func copySupplySetting(setting, existing resellerdomain.ProductSetting) resellerdomain.ProductSetting {
	setting.ID = existing.ID
	setting.ProductID = existing.ProductID
	setting.ResellerID = existing.ResellerID
	setting.SupplyPriceAmount = existing.SupplyPriceAmount
	return setting
}

func validateSupplySettings(profile *resellerdomain.Profile, product productdomain.Product, settings []resellerdomain.ProductSetting) error {
	var ps *resellerdomain.ProductSetting
	skus := map[uint]*resellerdomain.ProductSetting{}
	for i := range settings {
		setting := &settings[i]
		setting.ProductID = product.ID
		if setting.SKUID == 0 {
			ps = setting
		} else {
			skus[setting.SKUID] = setting
		}
	}
	validate := func(sku *productdomain.ProductSKU, retail, cost decimal.Decimal, ss *resellerdomain.ProductSetting) error {
		base := ResolveSupplyPrice(retail, ps, ss)
		if err := ValidateSupplyPrice(cost, base, ps, ss); err != nil {
			return err
		}
		if (ps != nil && !ps.IsListed) || (ss != nil && !ss.IsListed) {
			return nil
		}
		price, _, err := ResolveUnitAmount(profile, ps, ss, base)
		if err != nil {
			return err
		}
		if err := ValidateUnitAmount(profile, sku, base, price); err != nil {
			return err
		}
		if cost.IsPositive() && price.LessThan(cost.Round(2)) {
			return resellercontract.ErrPriceBelowBase
		}
		return nil
	}
	if len(product.SKUs) == 0 {
		return validate(nil, product.PriceAmount.Decimal, product.CostPriceAmount.Decimal, nil)
	}
	for i := range product.SKUs {
		sku := &product.SKUs[i]
		if !sku.IsActive {
			continue
		}
		if err := validate(sku, sku.PriceAmount.Decimal, sku.CostPriceAmount.Decimal, skus[sku.ID]); err != nil {
			return err
		}
	}
	return nil
}

func (s *ProductSettingService) previewSettings(profile *resellerdomain.Profile, productID uint, input ProductSettingSaveInput) ([]ProductSettingPreviewItem, error) {
	if profile == nil || productID == 0 {
		return nil, productcontract.ErrNotFound
	}
	row, err := s.store.GetProductWithSettings(profile.ID, productID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, productcontract.ErrNotFound
	}
	product := row.Product
	if err := validateSettingSKUs(product, input); err != nil {
		return nil, err
	}

	settings, err := mergeProductSettings(row.Settings, input)
	if err != nil {
		return nil, err
	}
	var productSetting *resellerdomain.ProductSetting
	skuSettings := map[uint]*resellerdomain.ProductSetting{}
	for i := range settings {
		setting := &settings[i]
		if setting.SKUID == 0 {
			productSetting = setting
		} else {
			skuSettings[setting.SKUID] = setting
		}
	}

	items := make([]ProductSettingPreviewItem, 0, len(product.SKUs)+1)

	productBase := ResolveSupplyPrice(product.PriceAmount.Decimal, productSetting, nil)
	productItem := ProductSettingPreviewItem{
		SKUID:     0,
		IsListed:  productSetting == nil || productSetting.IsListed,
		BasePrice: productBase,
		Valid:     true,
	}
	if productSetting != nil && productSetting.IsListed {
		price, _, _ := ResolveUnitAmount(profile, productSetting, nil, productBase)
		productItem.EffectivePrice = price.Round(2)
		if len(product.SKUs) == 0 {
			perr := ValidateUnitAmount(profile, nil, productBase, price)
			if perr == nil {
				cost := product.CostPriceAmount.Decimal.Round(2)
				if cost.GreaterThan(decimal.Zero) && price.LessThan(cost) {
					perr = resellercontract.ErrPriceBelowBase
				}
			}
			if perr == nil {
				perr = ValidateSupplyPrice(product.CostPriceAmount.Decimal, productBase, productSetting, nil)
			}
			productItem.Valid = perr == nil
			productItem.ErrorCode = previewErrorCode(perr)
		} else {
			err := validateSupplySettings(profile, product, settings)
			productItem.Valid, productItem.ErrorCode = err == nil, previewErrorCode(err)
		}
	}
	items = append(items, productItem)

	for i := range product.SKUs {
		sku := &product.SKUs[i]
		if !sku.IsActive {
			continue
		}
		skuSetting := skuSettings[sku.ID]
		base := ResolveSupplyPrice(sku.PriceAmount.Decimal, productSetting, skuSettings[sku.ID])
		item := ProductSettingPreviewItem{SKUID: sku.ID, IsListed: true, BasePrice: base, Valid: true}
		if (productSetting != nil && !productSetting.IsListed) || (skuSetting != nil && !skuSetting.IsListed) {
			item.IsListed = false
			items = append(items, item)
			continue
		}
		price, _, perr := ResolveUnitAmount(profile, productSetting, skuSetting, base)
		if perr == nil {
			perr = ValidateUnitAmount(profile, sku, base, price)
			if perr == nil {
				perr = ValidateSupplyPrice(sku.CostPriceAmount.Decimal, base, productSetting, skuSetting)
			}
		}
		item.EffectivePrice = price.Round(2)
		item.Valid = perr == nil
		item.ErrorCode = previewErrorCode(perr)
		items = append(items, item)
	}
	return items, nil
}

func buildPreviewSetting(input ProductSettingInput) (resellerdomain.ProductSetting, error) {
	mode := strings.TrimSpace(input.PricingMode)
	if mode == "" {
		mode = resellerdomain.PricingModeInherit
	}
	switch mode {
	case resellerdomain.PricingModeInherit, resellerdomain.PricingModeMarkupPercent, resellerdomain.PricingModeFixedMarkup, resellerdomain.PricingModeFixedPrice:
	default:
		return resellerdomain.ProductSetting{}, resellercontract.ErrPricingModeInvalid
	}
	return resellerdomain.ProductSetting{
		SKUID:             input.SKUID,
		IsListed:          input.IsListed,
		PricingMode:       mode,
		MarkupPercent:     money.FromDecimal(input.MarkupPercent.Round(2)),
		FixedMarkupAmount: money.FromDecimal(input.FixedMarkupAmount.Round(2)),
		FixedPriceAmount:  money.FromDecimal(input.FixedPriceAmount.Round(2)),
		SortOrder:         input.SortOrder,
	}, nil
}

func previewErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, resellercontract.ErrMarkupExceeded):
		return "markup_exceeded"
	default:
		return "price_invalid"
	}
}

func computeProductEffectivePrices(profile *resellerdomain.Profile, product productdomain.Product, settings []resellerdomain.ProductSetting) (map[uint]decimal.Decimal, map[uint]string, error) {
	effective := map[uint]decimal.Decimal{}
	rules := map[uint]string{}
	byProduct, bySKU := indexProductSettings(settings)
	productSetting := byProduct[product.ID]
	if productSetting != nil && productSetting.IsListed {
		price, rule, err := ResolveUnitAmount(profile, productSetting, nil, ResolveSupplyPrice(product.PriceAmount.Decimal, productSetting, nil))
		if err != nil {
			return effective, rules, err
		}
		effective[0] = price.Round(2)
		rules[0] = rule.Source
	}
	for i := range product.SKUs {
		sku := &product.SKUs[i]
		if !sku.IsActive {
			continue
		}
		skuSetting := bySKU[productSettingKey{productID: product.ID, skuID: sku.ID}]
		if productSetting != nil && !productSetting.IsListed {
			continue
		}
		if skuSetting != nil && !skuSetting.IsListed {
			continue
		}
		price, rule, err := ResolveUnitAmount(profile, productSetting, skuSetting, ResolveSupplyPrice(sku.PriceAmount.Decimal, productSetting, skuSetting))
		if err != nil {
			return effective, rules, err
		}
		effective[sku.ID] = price.Round(2)
		rules[sku.ID] = rule.Source
	}
	return effective, rules, nil
}

type productSettingKey struct {
	productID uint
	skuID     uint
}

func indexProductSettings(settings []resellerdomain.ProductSetting) (map[uint]*resellerdomain.ProductSetting, map[productSettingKey]*resellerdomain.ProductSetting) {
	byProduct := make(map[uint]*resellerdomain.ProductSetting)
	bySKU := make(map[productSettingKey]*resellerdomain.ProductSetting)
	for i := range settings {
		setting := settings[i]
		row := setting
		if setting.SKUID == 0 {
			byProduct[setting.ProductID] = &row
			continue
		}
		bySKU[productSettingKey{productID: setting.ProductID, skuID: setting.SKUID}] = &row
	}
	return byProduct, bySKU
}

func findProductSKU(items []productdomain.ProductSKU, skuID uint) *productdomain.ProductSKU {
	for i := range items {
		if items[i].ID == skuID {
			return &items[i]
		}
	}
	return nil
}

func validateSettingSKUs(product productdomain.Product, input ProductSettingSaveInput) error {
	for _, item := range input.Settings {
		if item.SKUID == 0 {
			continue
		}
		sku := findProductSKU(product.SKUs, item.SKUID)
		if sku == nil || (item.IsListed && !sku.IsActive) {
			return productcontract.ErrProductSKUInvalid
		}
	}
	return nil
}
