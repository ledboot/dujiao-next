package storefrontseo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	productcontract "github.com/dujiao-next/internal/modules/catalog/product/contract"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	contentapp "github.com/dujiao-next/internal/modules/content/application"
	contentcontract "github.com/dujiao-next/internal/modules/content/contract"
	contentdomain "github.com/dujiao-next/internal/modules/content/domain"
	reseller "github.com/dujiao-next/internal/modules/reseller/contract"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

type settingsStub struct{}

func (settingsStub) GetConfig(map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"brand": map[string]any{"site_name": "Brand", "site_url": "https://canonical.example/"}}, nil
}

type productsStub struct{ activeOnly bool }

func (p *productsStub) List(productcontract.ListFilter) ([]productdomain.Product, int64, error) {
	return nil, 0, nil
}
func (p *productsStub) GetBySlug(_ string, onlyActive bool) (*productdomain.Product, error) {
	p.activeOnly = onlyActive
	return &productdomain.Product{IsActive: true, TitleJSON: jsonmap.JSON{"zh-CN": "Public product"}, ContentJSON: jsonmap.JSON{"zh-CN": "<p>Public description</p>"}, InstructionsJSON: jsonmap.JSON{"zh-CN": "PRIVATE DELIVERY SECRET"}}, nil
}

type postsStub struct{}

func (postsStub) ListPublic(context.Context, contentapp.PublicPostQuery) ([]contentdomain.Post, int64, error) {
	return nil, 0, nil
}
func (postsStub) GetPublicBySlug(context.Context, string) (*contentdomain.Post, error) {
	return nil, contentcontract.ErrNotFound
}

type tenantStub struct{ tenant reseller.TenantContext }

func (t tenantStub) ResolveRequest(context.Context, *http.Request) (reseller.TenantContext, error) {
	return t.tenant, nil
}

func TestPublicProjectionOmitsDeliveryDataAndUsesConfiguredOrigin(t *testing.T) {
	p := &productsStub{}
	r := reader{settings: settingsStub{}, products: p}
	req := httptest.NewRequest("GET", "https://untrusted.example/products/item?ref=x", nil)
	page, err := r.load(req)
	if err != nil {
		t.Fatal(err)
	}
	if !p.activeOnly {
		t.Fatal("product must be queried with publication checks")
	}
	if page.Canonical != "https://canonical.example/products/item" {
		t.Fatal(page.Canonical)
	}
	data, _ := json.Marshal(page)
	if strings.Contains(string(data), "PRIVATE") || page.Body != "<p>Public description</p>" {
		t.Fatalf("incorrect public projection: %s", data)
	}
}

func TestDraftPrivateAndTenantBoundaries(t *testing.T) {
	r := reader{settings: settingsStub{}, posts: postsStub{}}
	page, err := r.load(httptest.NewRequest("GET", "https://shop.example/blog/draft", nil))
	if err != nil || page.Status != 404 {
		t.Fatalf("draft: %v %v", page, err)
	}
	for _, path := range []string{"/cart", "/me/orders", "/auth/login", "/guest/orders", "/orders/secret"} {
		page, err = r.load(httptest.NewRequest("GET", "https://shop.example"+path, nil))
		if err != nil || page != nil {
			t.Fatalf("private route %s rendered", path)
		}
	}
	r.settings = nil // A reseller must never even read main-site settings.
	r.tenants = tenantStub{reseller.ResellerTenantContext("reseller.example", 42, 1, "reseller.example")}
	page, err = r.load(httptest.NewRequest("GET", "https://reseller.example/", nil))
	if err != nil || page != nil {
		t.Fatal("reseller leaked main storefront")
	}
	for _, path := range []string{"/about", "/terms", "/privacy", "/blog/main-only"} {
		page, err = r.load(httptest.NewRequest("GET", "https://reseller.example"+path, nil))
		if err != nil || page == nil || page.Status != 404 || page.Body != "" || page.Canonical != "" {
			t.Fatalf("reseller unowned page %s: %+v %v", path, page, err)
		}
	}
	r.tenants = tenantStub{reseller.UnavailableTenantContext("unknown.example", "unknown")}
	page, err = r.load(httptest.NewRequest("GET", "https://unknown.example/", nil))
	if err != nil || page.Status != 404 {
		t.Fatal("unknown host must be unavailable")
	}
}
