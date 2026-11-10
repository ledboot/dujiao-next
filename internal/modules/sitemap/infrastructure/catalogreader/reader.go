package catalogreader

import (
	"context"
	"fmt"
	categorycontract "github.com/dujiao-next/internal/modules/catalog/category/contract"
	productcontract "github.com/dujiao-next/internal/modules/catalog/product/contract"
	reseller "github.com/dujiao-next/internal/modules/reseller/contract"
	"github.com/dujiao-next/internal/modules/sitemap/contract"
)

// Reader 将 Catalog 的仓储契约收窄为 Sitemap 投影。
type Reader struct {
	products   productcontract.Repository
	categories categorycontract.Repository
	hidden     interface{ ListHiddenProductIDs(uint) ([]uint, error) }
}

var _ contract.CatalogReader = (*Reader)(nil)

// New 创建 Catalog 读取适配器。
func New(products productcontract.Repository, categories categorycontract.Repository, hidden ...interface{ ListHiddenProductIDs(uint) ([]uint, error) }) *Reader {
	if products == nil || categories == nil {
		panic("sitemap catalog reader: required repository is nil")
	}
	r := &Reader{products: products, categories: categories}
	if len(hidden) > 0 {
		r.hidden = hidden[0]
	}
	return r
}

// ListActiveCategories 返回可索引分类投影。
func (r *Reader) ListActiveCategories(ctx context.Context) ([]contract.Category, error) {
	rows, err := r.categories.ListActive()
	if err != nil {
		return nil, err
	}
	result := make([]contract.Category, 0, len(rows))
	visible := map[uint]bool{}
	if reseller.TenantOrMain(ctx).IsReseller() {
		filter, err := r.productFilter(ctx, 50000)
		if err != nil {
			return nil, err
		}
		products, _, err := r.products.List(filter)
		if err != nil {
			return nil, err
		}
		parents := map[uint]uint{}
		for _, row := range rows {
			parents[row.ID] = row.ParentID
		}
		for _, product := range products {
			for id := product.CategoryID; id != 0 && !visible[id]; id = parents[id] {
				visible[id] = true
			}
		}
	}
	for _, row := range rows {
		if reseller.TenantOrMain(ctx).IsReseller() && !visible[row.ID] {
			continue
		}
		result = append(result, contract.Category{Slug: row.Slug, CreatedAt: row.CreatedAt})
	}
	return result, nil
}

// ListActiveProducts 返回可索引商品投影。
func (r *Reader) productFilter(ctx context.Context, limit int) (productcontract.ListFilter, error) {
	filter := productcontract.ListFilter{
		Page:       1,
		PageSize:   limit,
		OnlyActive: true,
	}
	if tenant := reseller.TenantOrMain(ctx); tenant.IsReseller() {
		if r.hidden == nil {
			return filter, fmt.Errorf("sitemap: reseller visibility reader unavailable")
		}
		ids, err := r.hidden.ListHiddenProductIDs(*tenant.ResellerID)
		if err != nil {
			return filter, err
		}
		filter.ExcludeProductIDs = ids
	}
	return filter, nil
}

func (r *Reader) ListActiveProducts(ctx context.Context, limit int) ([]contract.Product, error) {
	filter, err := r.productFilter(ctx, limit)
	if err != nil {
		return nil, err
	}
	rows, _, err := r.products.List(filter)
	if err != nil {
		return nil, err
	}
	result := make([]contract.Product, 0, len(rows))
	for _, row := range rows {
		result = append(result, contract.Product{Slug: row.Slug, UpdatedAt: row.UpdatedAt})
	}
	return result, nil
}
