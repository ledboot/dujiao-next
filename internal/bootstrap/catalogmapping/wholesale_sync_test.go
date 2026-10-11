package catalogmappingbootstrap

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	mappingdomain "github.com/dujiao-next/internal/modules/catalog/mapping/domain"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/dujiao-next/internal/upstream"
	"github.com/shopspring/decimal"
)

func TestWholesaleSyncSwitchAcrossSyncAndImport(t *testing.T) {
	for _, mode := range []string{"manual", "scheduled", "import"} {
		for _, enabled := range []bool{true, false} {
			for _, empty := range []bool{false, true} {
				name := fmt.Sprintf("%s/enabled=%v/empty=%v", mode, enabled, empty)
				t.Run(name, func(t *testing.T) {
					upProduct := upstream.UpstreamProduct{
						ID: 101, Title: jsonmap.JSON{"zh-CN": "批发测试商品"}, IsActive: true, PriceAmount: "10.00", Currency: "CNY",
						SKUs:            []upstream.UpstreamSKU{{ID: 201, SKUCode: "SKU-A", PriceAmount: "10.00", IsActive: true, StockQuantity: 42}},
						WholesalePrices: productdomain.WholesalePriceTiers{},
					}
					if !empty {
						upProduct.WholesalePrices = productdomain.WholesalePriceTiers{{SKUID: 201, MinQuantity: 5, UnitPrice: money.FromDecimal(decimal.NewFromInt(8))}}
					}
					svc, db, mapping, cleanup := setupMappingWithUpstreamHandler(t, fmt.Sprintf("file:wholesale_%s_%v_%v?mode=memory&cache=shared", mode, enabled, empty), func(w http.ResponseWriter, r *http.Request) {
						if mode == "scheduled" {
							listProductsHandler([]upstream.UpstreamProduct{upProduct}, true)(w, r)
							return
						}
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "product": upProduct})
					})
					defer cleanup()
					sqlDB, err := db.DB()
					if err != nil {
						t.Fatal(err)
					}
					defer sqlDB.Close()
					sqlDB.SetMaxOpenConns(1)
					if err := db.Model(&siteconnectiondomain.Connection{}).Where("id = ?", mapping.ConnectionID).Updates(map[string]any{
						"sync_wholesale_price": enabled, "auto_sync_price": false, "exchange_rate": 2, "price_markup_percent": 25,
					}).Error; err != nil {
						t.Fatal(err)
					}
					original := productdomain.WholesalePriceTiers{{MinQuantity: 10, UnitPrice: money.FromDecimal(decimal.NewFromInt(7))}}
					if err := db.Model(&productdomain.Product{}).Where("id = ?", mapping.LocalProductID).Update("wholesale_prices", original).Error; err != nil {
						t.Fatal(err)
					}
					productID := mapping.LocalProductID
					switch mode {
					case "manual":
						err = svc.SyncProduct(mapping.ID)
					case "scheduled":
						err = svc.SyncConnectionStock(mapping.ConnectionID, []mappingdomain.Mapping{*mapping}, 50, 200)
					case "import":
						// 同一连接另一个上游商品的首次导入。
						upProduct.ID = 102
						imported, importErr := svc.ImportUpstreamProduct(mapping.ConnectionID, 102, 1, "imported-wholesale")
						err = importErr
						if err == nil {
							productID = imported.LocalProductID
						}
					}
					if err != nil {
						t.Fatal(err)
					}
					var product productdomain.Product
					if err := db.First(&product, productID).Error; err != nil {
						t.Fatal(err)
					}
					if !enabled && mode != "import" {
						if len(product.WholesalePrices) != 1 || product.WholesalePrices[0].MinQuantity != 10 || product.WholesalePrices[0].UnitPrice.String() != "7.00" || product.WholesalePrices[0].SKUID != 0 {
							t.Fatalf("local tiers changed while disabled: %+v", product.WholesalePrices)
						}
					} else if empty || !enabled {
						if len(product.WholesalePrices) != 0 {
							t.Fatalf("expected no wholesale tiers, got %+v", product.WholesalePrices)
						}
					} else {
						if len(product.WholesalePrices) != 1 {
							t.Fatalf("expected replacement tier, got %+v", product.WholesalePrices)
						}
						tier := product.WholesalePrices[0]
						var sku productdomain.ProductSKU
						if err := db.Where("product_id = ? AND sku_code = ?", productID, "SKU-A").First(&sku).Error; err != nil {
							t.Fatal(err)
						}
						if tier.MinQuantity != 5 || tier.UnitPrice.String() != "20.00" || tier.SKUID != sku.ID || tier.SKUCode != sku.SKUCode {
							t.Fatalf("unexpected converted replacement: %+v", tier)
						}
					}
					if mode != "import" && product.PriceAmount.String() != "10.00" {
						t.Fatalf("wholesale sync must not enable retail sync: %s", product.PriceAmount.String())
					}
				})
			}
		}
	}
}
