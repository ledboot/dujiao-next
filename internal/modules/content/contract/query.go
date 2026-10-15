package contract

import "context"

type postOwnerKey struct{}

// WithPostOwner scopes authenticated announcement management to a reseller.
func WithPostOwner(ctx context.Context, id uint) context.Context {
	return context.WithValue(ctx, postOwnerKey{}, id)
}

func PostOwner(ctx context.Context) uint {
	id, _ := ctx.Value(postOwnerKey{}).(uint)
	return id
}

// PostOrder 限定文章列表可使用的稳定排序策略，避免向持久化层传递任意 SQL。
type PostOrder uint8

const (
	PostOrderCreatedDesc PostOrder = iota
	PostOrderPublishedDesc
)

// PostQuery 描述文章列表查询。
type PostQuery struct {
	ResellerID    uint
	Page          int
	PageSize      int
	Type          string
	Search        string
	OnlyPublished bool
	Order         PostOrder
}

// BannerQuery 描述后台 Banner 列表查询。
type BannerQuery struct {
	Page     int
	PageSize int
	Position string
	Search   string
	IsActive *bool
}

// MediaQuery 描述素材列表查询。
type MediaQuery struct {
	Page     int
	PageSize int
	Scene    string
	Search   string
}
