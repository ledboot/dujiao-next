// Package storefrontseo assembles public HTML projections at the composition
// boundary. It never exposes delivery instructions, account data or prices.
package storefrontseo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/htmltext"
	"github.com/dujiao-next/internal/i18n"
	categorydomain "github.com/dujiao-next/internal/modules/catalog/category/domain"
	productcontract "github.com/dujiao-next/internal/modules/catalog/product/contract"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	contentapp "github.com/dujiao-next/internal/modules/content/application"
	contentcontract "github.com/dujiao-next/internal/modules/content/contract"
	contentdomain "github.com/dujiao-next/internal/modules/content/domain"
	reseller "github.com/dujiao-next/internal/modules/reseller/contract"
	"github.com/dujiao-next/internal/web"
)

type settingsReader interface {
	GetConfig(map[string]interface{}) (map[string]interface{}, error)
}
type productReader interface {
	List(productcontract.ListFilter) ([]productdomain.Product, int64, error)
	GetBySlug(string, bool) (*productdomain.Product, error)
}
type categoryReader interface {
	ListActive() ([]categorydomain.Category, error)
	GetBySlug(string) (*categorydomain.Category, error)
}
type postReader interface {
	ListPublic(context.Context, contentapp.PublicPostQuery) ([]contentdomain.Post, int64, error)
	GetPublicBySlug(context.Context, string) (*contentdomain.Post, error)
}
type tenantResolver interface {
	ResolveRequest(context.Context, *http.Request) (reseller.TenantContext, error)
}

type reader struct {
	settings   settingsReader
	products   productReader
	categories categoryReader
	posts      postReader
	tenants    tenantResolver
}

func New(c *container.Container) web.PageLoader {
	r := reader{settings: c.SettingService, products: c.ProductRepo, categories: c.CategoryRepo, posts: c.ContentPostService}
	if c.ResellerDomainResolver != nil {
		r.tenants = c.ResellerDomainResolver
	}
	return r.load
}

func (r reader) load(req *http.Request) (*web.PublicPage, error) {
	path := req.URL.Path
	// Private SPA routes keep their existing authentication and captcha behavior.
	for _, prefix := range []string{"/auth/", "/me", "/guest/", "/orders/", "/recharge-orders/", "/reseller/"} {
		if path == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(path, strings.TrimSuffix(prefix, "/")+"/") {
			return nil, nil
		}
	}
	if path == "/cart" || path == "/checkout" || path == "/pay" {
		return nil, nil
	}
	if r.tenants != nil {
		tenant, err := r.tenants.ResolveRequest(req.Context(), req)
		if err != nil {
			return nil, err
		}
		if tenant.Unavailable {
			return &web.PublicPage{Path: path, Locale: i18n.LocaleZH, Status: http.StatusNotFound}, nil
		}
		// Resellers retain their storefront; do not render main-site catalog or branding.
		if !tenant.IsMain || tenant.ResellerID != nil {
			if path == "/about" || path == "/terms" || path == "/privacy" {
				return &web.PublicPage{Path: path, Locale: i18n.LocaleZH, Status: http.StatusNotFound}, nil
			}
			if strings.HasPrefix(path, "/blog/") {
				ctx := reseller.WithTenantContext(req.Context(), tenant)
				_, err := r.posts.GetPublicBySlug(ctx, strings.TrimPrefix(path, "/blog/"))
				if errors.Is(err, contentcontract.ErrNotFound) {
					return &web.PublicPage{Path: path, Locale: i18n.LocaleZH, Status: http.StatusNotFound}, nil
				}
				if err != nil {
					return nil, err
				}
			}
			return nil, nil
		}
	}
	cfg, err := r.settings.GetConfig(nil)
	if err != nil {
		return nil, err
	}
	// Stable default language for indexable URLs; explicit variants remain usable.
	lang := req.URL.Query().Get("lang")
	if lang != i18n.LocaleEN && lang != i18n.LocaleTW {
		lang = i18n.LocaleZH
	}
	brand, seo := object(cfg["brand"]), object(cfg["seo"])
	base, err := url.Parse(text(brand["site_url"], lang))
	if err != nil || base.Host == "" || base.User != nil || (base.Scheme != "https" && base.Scheme != "http") {
		// Existing installations without a canonical origin keep the SPA behavior.
		// Do not build a canonical URL from an untrusted Host header.
		return nil, nil
	}
	base.Path = "/"
	base.RawPath = ""
	base.RawQuery = ""
	base.Fragment = ""
	canonical := base.ResolveReference(&url.URL{Path: path}).String()
	page := &web.PublicPage{Path: path, Locale: lang, SiteName: text(brand["site_name"], lang), Type: "WebPage", Canonical: canonical, ContinueLabel: i18n.T(lang, "seo.continue_shopping")}
	page.Heading = page.SiteName
	page.Description = text(seo["description"], lang)
	if page.Description == "" {
		page.Description = text(brand["site_description"], lang)
	}
	if raw := text(seo["default_og_image"], lang); raw != "" {
		page.Image = absoluteImage(raw, base)
	}
	if page.Image == "" {
		page.Image = absoluteImage(text(brand["site_logo"], lang), base)
	}
	page.Navigation = []web.PublicLink{{Title: page.SiteName, URL: "/"}, {Title: i18n.T(lang, "seo.products"), URL: "/products"}, {Title: i18n.T(lang, "seo.guides"), URL: "/blog"}, {Title: i18n.T(lang, "seo.about"), URL: "/about"}}
	missing := func() (*web.PublicPage, error) {
		page.Status = http.StatusNotFound
		page.Heading = i18n.T(lang, "error.post_not_found")
		page.Title = page.Heading
		page.Description = ""
		return page, nil
	}
	switch {
	case path == "/":
		page.Title = text(seo["title"], lang)
		page.Body = text(object(cfg["about"])["introduction"], lang)
		posts, _, listErr := r.posts.ListPublic(req.Context(), contentapp.PublicPostQuery{Type: "blog", Page: 1, PageSize: 20})
		if listErr != nil {
			return nil, listErr
		}
		for _, post := range posts {
			page.Links = append(page.Links, postLink(post, lang))
		}
	case path == "/blog" || path == "/notice":
		postType := "blog"
		key := "seo.guides"
		if path == "/notice" {
			postType = "notice"
			key = "seo.notices"
		}
		page.Heading = i18n.T(lang, key)
		posts, _, listErr := r.posts.ListPublic(req.Context(), contentapp.PublicPostQuery{Type: postType, Page: 1, PageSize: 100})
		if listErr != nil {
			return nil, listErr
		}
		for _, post := range posts {
			page.Links = append(page.Links, postLink(post, lang))
		}
	case strings.HasPrefix(path, "/blog/"):
		slug := strings.TrimPrefix(path, "/blog/")
		if slug == "" || strings.Contains(slug, "/") {
			return missing()
		}
		post, getErr := r.posts.GetPublicBySlug(req.Context(), slug)
		if errors.Is(getErr, contentcontract.ErrNotFound) || (getErr == nil && post == nil) {
			return missing()
		}
		if getErr != nil {
			return nil, getErr
		}
		page.Heading = text(post.TitleJSON, lang)
		page.Description = plain(text(post.SummaryJSON, lang))
		page.Body = text(post.ContentJSON, lang)
		page.Type = "Article"
		if post.PublishedAt != nil {
			page.PublishedAt = post.PublishedAt.UTC().Format(time.RFC3339)
		}
		if post.Thumbnail != "" {
			page.Image = absoluteImage(post.Thumbnail, base)
		}
	case path == "/products" || strings.HasPrefix(path, "/categories/"):
		filter := productcontract.ListFilter{Page: 1, PageSize: 100, OnlyActive: true}
		page.Heading = i18n.T(lang, "seo.products")
		if strings.HasPrefix(path, "/categories/") {
			cat, getErr := r.categories.GetBySlug(strings.TrimPrefix(path, "/categories/"))
			if getErr != nil {
				return nil, getErr
			}
			if cat == nil || !cat.IsActive || cat.DeletedAt != nil {
				return missing()
			}
			page.Heading = text(cat.NameJSON, lang)
			filter.CategoryID = strconv.FormatUint(uint64(cat.ID), 10)
		} else {
			cats, listErr := r.categories.ListActive()
			if listErr != nil {
				return nil, listErr
			}
			for _, cat := range cats {
				page.Links = append(page.Links, web.PublicLink{Title: text(cat.NameJSON, lang), URL: "/categories/" + url.PathEscape(cat.Slug)})
			}
		}
		products, _, listErr := r.products.List(filter)
		if listErr != nil {
			return nil, listErr
		}
		for _, p := range products {
			page.Links = append(page.Links, web.PublicLink{Title: text(p.TitleJSON, lang), Description: plain(text(p.DescriptionJSON, lang)), URL: "/products/" + url.PathEscape(p.Slug)})
		}
	case strings.HasPrefix(path, "/products/"):
		slug := strings.TrimPrefix(path, "/products/")
		if slug == "" || strings.Contains(slug, "/") {
			return missing()
		}
		p, getErr := r.products.GetBySlug(slug, true)
		if getErr != nil {
			return nil, getErr
		}
		if p == nil || !p.IsActive || p.DeletedAt != nil {
			return missing()
		}
		page.Heading = text(p.TitleJSON, lang)
		page.Description = plain(text(p.SeoMetaJSON["description"], lang))
		if page.Description == "" {
			page.Description = plain(text(p.DescriptionJSON, lang))
		}
		page.Body = text(p.ContentJSON, lang)
		page.Type = "Product"
		if len(p.Images) > 0 {
			page.Image = absoluteImage(p.Images[0], base)
		}
	case path == "/about":
		about := object(cfg["about"])
		page.Heading = text(object(about["hero"])["title"], lang)
		if page.Heading == "" {
			page.Heading = i18n.T(lang, "seo.about")
		}
		page.Body = text(about["introduction"], lang)
		if strings.TrimSpace(page.Body) == "" {
			return missing()
		}
	case path == "/terms" || path == "/privacy":
		key := strings.TrimPrefix(path, "/")
		page.Heading = i18n.T(lang, "seo."+key)
		page.Body = text(object(cfg["legal"])[key], lang)
		if strings.TrimSpace(page.Body) == "" {
			return missing()
		}
	default:
		return missing()
	}
	if page.Title == "" {
		page.Title = page.Heading
		if page.Heading != page.SiteName {
			page.Title += " - " + page.SiteName
		}
	}
	return page, nil
}

func object(value any) map[string]any {
	// Settings and domain JSON have distinct named map types.
	data, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(data, &result)
	return result
}
func text(value any, locale string) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	m := object(value)
	for _, key := range []string{locale, i18n.LocaleZH, i18n.LocaleEN, i18n.LocaleTW} {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
func plain(raw string) string {
	return strings.Join(strings.Fields(htmltext.StripToPlainText(raw)), " ")
}
func postLink(p contentdomain.Post, lang string) web.PublicLink {
	return web.PublicLink{Title: text(p.TitleJSON, lang), Description: plain(text(p.SummaryJSON, lang)), URL: "/blog/" + url.PathEscape(p.Slug)}
}
func absoluteImage(raw string, base *url.URL) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return ""
	}
	u = base.ResolveReference(u)
	if u.Scheme != "https" && u.Scheme != "http" {
		return ""
	}
	return u.String()
}
