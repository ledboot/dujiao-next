package web

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/html"
)

// PageLoader exposes only an explicitly public projection. It must enforce tenant
// and publication boundaries before returning any content.
type PageLoader func(*http.Request) (*PublicPage, error)

type PublicLink struct{ Title, Description, URL string }

type PublicPage struct {
	Path, Locale, SiteName, Title, Heading, Description, Canonical, Image string
	Body, PublishedAt, Type                                               string
	Links                                                                 []PublicLink
	Navigation                                                            []PublicLink
	ContinueLabel                                                         string
	Status                                                                int
}

var titleElement = regexp.MustCompile(`(?is)<title[^>]*>.*?</title>`)
var htmlLanguage = regexp.MustCompile(`(?i)<html\s+lang="[^"]*"`)

var publicHead = template.Must(template.New("public-head").Parse(`<title data-storefront-seo>{{.Title}}</title>
<meta data-storefront-seo name="description" content="{{.Description}}">
<meta data-storefront-seo name="robots" content="{{if .Hidden}}noindex, follow{{else}}index, follow, max-image-preview:large{{end}}">
{{if not .Hidden}}<link data-storefront-seo rel="canonical" href="{{.Canonical}}">{{end}}
<meta data-storefront-seo property="og:site_name" content="{{.SiteName}}">
<meta data-storefront-seo property="og:title" content="{{.Title}}">
<meta data-storefront-seo property="og:description" content="{{.Description}}">
<meta data-storefront-seo property="og:type" content="{{if eq .Type "Article"}}article{{else}}website{{end}}">
<meta data-storefront-seo property="og:url" content="{{.Canonical}}">
{{if .Image}}<meta data-storefront-seo property="og:image" content="{{.Image}}">{{end}}
<script data-storefront-seo type="application/ld+json">{{.Schema}}</script>
<script id="storefront-public-page" type="application/json">{{.Snapshot}}</script>
`))

var publicBody = template.Must(template.New("public-body").Parse(`<section id="storefront-public-content" lang="{{.Locale}}" aria-label="{{.Heading}}">
<style>#storefront-public-content{color:#17202a;background:#fff;font:16px/1.8 system-ui,sans-serif;padding:28px 20px;overflow-wrap:anywhere}#storefront-public-content>div{max-width:900px;margin:auto}#storefront-public-content h1{font-size:clamp(24px,5vw,36px);line-height:1.35}#storefront-public-content h2{font-size:22px;margin-top:28px}#storefront-public-content a{color:#075db8;text-decoration:underline}#storefront-public-content nav{display:flex;gap:16px;flex-wrap:wrap}#storefront-public-content img{max-width:100%;height:auto}#storefront-public-content li{margin:10px 0}#storefront-public-content time{font-size:14px}@media(prefers-color-scheme:dark){#storefront-public-content{background:#111827;color:#e5e7eb}#storefront-public-content a{color:#93c5fd}}</style>
<div><nav>{{range .Navigation}}<a href="{{.URL}}">{{.Title}}</a>{{end}}</nav>
<article><h1>{{.Heading}}</h1>{{if .PublishedAt}}<time datetime="{{.PublishedAt}}">{{.PublishedAt}}</time>{{end}}
{{if .Description}}<p>{{.Description}}</p>{{end}}{{.SafeBody}}
{{if .Links}}<ul>{{range .Links}}<li><a href="{{.URL}}">{{.Title}}</a>{{if .Description}}<p>{{.Description}}</p>{{end}}</li>{{end}}</ul>{{end}}
</article><p><a href="#app">{{.ContinueLabel}}</a></p></div></section>
`))

func returnRenderError(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Robots-Tag", "noindex")
	c.Status(http.StatusServiceUnavailable)
}

func siteOrigin(canonical string) string {
	parsed, err := url.Parse(canonical)
	if err != nil || parsed.Host == "" {
		return strings.TrimRight(canonical, "/")
	}
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func alternateSiteName(name string) string {
	name = strings.TrimSpace(name)
	if len(name) <= len("AI") || !strings.HasSuffix(name, "AI") {
		return ""
	}
	prefix := strings.TrimSpace(strings.TrimSuffix(name, "AI"))
	if prefix == "" {
		return ""
	}
	return prefix + " AI"
}

func renderPublicPage(index []byte, page PublicPage) ([]byte, error) {
	hidden := page.Status >= 400
	origin := siteOrigin(page.Canonical)
	organizationID := origin + "/#organization"
	websiteID := origin + "/#website"
	pageID := page.Canonical + "#webpage"

	organization := map[string]any{
		"@type":       "Organization",
		"@id":         organizationID,
		"name":        page.SiteName,
		"url":         origin + "/",
		"description": page.Description,
		"contactPoint": map[string]any{
			"@type":             "ContactPoint",
			"contactType":       "customer service",
			"url":               origin + "/about",
			"availableLanguage": []string{"zh-CN", "zh-TW", "en"},
		},
	}
	if alias := alternateSiteName(page.SiteName); alias != "" {
		organization["alternateName"] = alias
	}
	if page.Image != "" {
		organization["logo"] = page.Image
	}

	website := map[string]any{
		"@type":       "WebSite",
		"@id":         websiteID,
		"name":        page.SiteName,
		"url":         origin + "/",
		"description": page.Description,
		"inLanguage":  page.Locale,
		"publisher":   map[string]any{"@id": organizationID},
	}
	if alias := alternateSiteName(page.SiteName); alias != "" {
		website["alternateName"] = alias
	}

	graph := []any{organization, website}
	if page.Path != "/" {
		entityType := page.Type
		if entityType == "" || entityType == "WebSite" {
			entityType = "WebPage"
		}
		if page.Path == "/about" {
			entityType = "AboutPage"
		}
		entity := map[string]any{
			"@type":       entityType,
			"@id":         pageID,
			"name":        page.Heading,
			"url":         page.Canonical,
			"description": page.Description,
			"inLanguage":  page.Locale,
			"isPartOf":    map[string]any{"@id": websiteID},
			"publisher":   map[string]any{"@id": organizationID},
		}
		if page.Path == "/about" {
			entity["about"] = map[string]any{"@id": organizationID}
			entity["mainEntity"] = map[string]any{"@id": organizationID}
		}
		if page.Image != "" {
			entity["image"] = page.Image
		}
		if entityType == "Article" {
			entity["headline"] = page.Heading
			entity["mainEntityOfPage"] = page.Canonical
			if page.PublishedAt != "" {
				entity["datePublished"] = page.PublishedAt
			}
		}
		graph = append(graph, entity)
	}
	schema, err := json.Marshal(map[string]any{"@context": "https://schema.org", "@graph": graph})
	if err != nil {
		return nil, err
	}
	snapshot, err := json.Marshal(map[string]any{"path": page.Path, "locale": page.Locale, "title": page.Title, "description": page.Description, "canonical": page.Canonical, "image": page.Image, "siteName": page.SiteName, "type": page.Type, "hidden": hidden, "schema": json.RawMessage(schema)})
	if err != nil {
		return nil, err
	}
	data := struct {
		PublicPage
		Hidden           bool
		Schema, Snapshot template.JS
		SafeBody         template.HTML
	}{page, hidden, template.JS(schema), template.JS(snapshot), template.HTML(sanitizePublicHTML(page.Body))}
	var head, body bytes.Buffer
	if err := publicHead.Execute(&head, data); err != nil {
		return nil, err
	}
	if err := publicBody.Execute(&body, data); err != nil {
		return nil, err
	}
	result := titleElement.ReplaceAll(index, nil)
	result = htmlLanguage.ReplaceAll(result, []byte(`<html lang="`+page.Locale+`"`))
	result = bytes.Replace(result, []byte("</head>"), append(head.Bytes(), []byte("</head>")...), 1)
	result = bytes.Replace(result, []byte(`<div id="app">`), append(body.Bytes(), []byte(`<div id="app">`)...), 1)
	return result, nil
}

// Keep rich text readable without trusting merchant HTML, URL schemes, forms,
// scripts or event attributes. Only this sanitized result enters template.HTML.
func sanitizePublicHTML(raw string) string {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return ""
	}
	allowed := map[string]bool{"p": true, "h2": true, "h3": true, "h4": true, "ul": true, "ol": true, "li": true, "strong": true, "em": true, "b": true, "i": true, "blockquote": true, "code": true, "pre": true, "a": true, "br": true, "img": true, "table": true, "thead": true, "tbody": true, "tr": true, "td": true, "th": true, "hr": true}
	drop := map[string]bool{"script": true, "style": true, "iframe": true, "object": true, "embed": true, "svg": true, "math": true, "form": true, "input": true, "button": true, "textarea": true, "select": true, "template": true}
	var clean func(*html.Node, *html.Node)
	clean = func(src, dst *html.Node) {
		if src.Type == html.TextNode {
			dst.AppendChild(&html.Node{Type: html.TextNode, Data: src.Data})
			return
		}
		if src.Type == html.ElementNode && drop[src.Data] {
			return
		}
		next := dst
		if src.Type == html.ElementNode && allowed[src.Data] {
			node := &html.Node{Type: html.ElementNode, Data: src.Data, DataAtom: src.DataAtom}
			for _, attr := range src.Attr {
				if attr.Namespace != "" {
					continue
				}
				if (src.Data == "a" && attr.Key == "href") || (src.Data == "img" && attr.Key == "src") {
					if safePublicURL(attr.Val) {
						node.Attr = append(node.Attr, html.Attribute{Key: attr.Key, Val: strings.TrimSpace(attr.Val)})
					}
				} else if attr.Key == "alt" || attr.Key == "title" {
					node.Attr = append(node.Attr, attr)
				}
			}
			if src.Data == "a" {
				node.Attr = append(node.Attr, html.Attribute{Key: "rel", Val: "noopener noreferrer"})
			}
			dst.AppendChild(node)
			next = node
		}
		for child := src.FirstChild; child != nil; child = child.NextSibling {
			clean(child, next)
		}
	}
	root := &html.Node{Type: html.ElementNode, Data: "div"}
	clean(doc, root)
	var out bytes.Buffer
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		_ = html.Render(&out, child)
	}
	return out.String()
}

func safePublicURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && u.User == nil && (u.Scheme == "" || u.Scheme == "https" || u.Scheme == "http") && !strings.HasPrefix(strings.TrimSpace(raw), "//") && !strings.Contains(raw, "\\")
}
