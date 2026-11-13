package domain

import (
	"strings"
	"time"

	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	"github.com/dujiao-next/internal/shared/money"

	"github.com/shopspring/decimal"
)

// Order 订单表
type Order struct {
	CustomerFeeAmount       money.Amount `gorm:"-" json:"fee_amount"`                                                              // 用户承担手续费的查询视图，不参与订单结算
	PaymentTaxAmount        money.Amount `gorm:"-" json:"payment_tax_amount"`                                                      // 手续费税额的查询视图
	ID                      uint         `gorm:"primarykey" json:"id"`                                                             // 主键
	OrderNo                 string       `gorm:"uniqueIndex;not null" json:"order_no"`                                             // 订单编号
	ParentID                *uint        `gorm:"index;index:idx_orders_risk_pending,priority:4" json:"parent_id,omitempty"`        // 父订单ID
	UserID                  uint         `gorm:"index;not null;index:idx_orders_risk_pending,priority:2" json:"user_id,omitempty"` // 用户ID（游客订单为 0）
	GuestEmail              string       `gorm:"index" json:"guest_email,omitempty"`                                               // 游客邮箱
	GuestPassword           string       `gorm:"type:varchar(200)" json:"-"`                                                       // 游客订单密码
	GuestLocale             string       `gorm:"type:varchar(20)" json:"guest_locale,omitempty"`                                   // 游客语言
	Status                  string       `gorm:"index;not null;index:idx_orders_risk_pending,priority:3" json:"status"`            // 订单状态
	Currency                string       `gorm:"not null" json:"currency"`                                                         // 币种
	OriginalAmount          money.Amount `gorm:"type:decimal(20,2);not null;default:0" json:"original_amount"`                     // 原始金额
	DiscountAmount          money.Amount `gorm:"type:decimal(20,2);not null;default:0" json:"discount_amount"`                     // 优惠金额
	MemberDiscountAmount    money.Amount `gorm:"type:decimal(20,2);not null;default:0" json:"member_discount_amount"`              // 会员优惠金额
	PromotionDiscountAmount money.Amount `gorm:"type:decimal(20,2);not null;default:0" json:"promotion_discount_amount"`           // 活动价优惠金额
	WholesaleDiscountAmount money.Amount `gorm:"type:decimal(20,2);not null;default:0" json:"wholesale_discount_amount"`           // 批发价优惠金额
	InvoiceRequested        bool         `gorm:"not null;default:false" json:"invoice_requested"`                                  // 是否开票（兼容字段，与 InvoiceType 同步）
	InvoiceType             string       `gorm:"type:varchar(16);not null;default:''" json:"invoice_type,omitempty"`               // 开票类型：general 普票 3% / special 专票 5%，空表示不开票
	TaxAmount               money.Amount `gorm:"type:decimal(20,2);not null;default:0" json:"tax_amount"`
	TotalAmount             money.Amount `gorm:"type:decimal(20,2);not null;default:0" json:"total_amount"`                // 实付金额
	WalletPaidAmount        money.Amount `gorm:"type:decimal(20,2);not null;default:0" json:"wallet_paid_amount"`          // 钱包支付金额
	OnlinePaidAmount        money.Amount `gorm:"type:decimal(20,2);not null;default:0" json:"online_paid_amount"`          // 在线支付金额
	RefundedAmount          money.Amount `gorm:"type:decimal(20,2);not null;default:0" json:"refunded_amount"`             // 已退款金额（退回钱包）
	MemberLevelID           *uint        `gorm:"index" json:"member_level_id,omitempty"`                                   // 下单时等级快照
	CouponID                *uint        `gorm:"index" json:"coupon_id,omitempty"`                                         // 优惠券ID
	PromotionID             *uint        `gorm:"index" json:"promotion_id,omitempty"`                                      // 活动价ID（单品订单）
	AffiliateProfileID      *uint        `gorm:"index" json:"affiliate_profile_id,omitempty"`                              // 推广返利关联用户ID快照
	AffiliateCode           string       `gorm:"type:varchar(32);index" json:"affiliate_code,omitempty"`                   // 推广返利联盟ID快照
	ResellerID              *uint        `gorm:"index" json:"reseller_id,omitempty"`                                       // 分销商ID，主站订单为 NULL
	ResellerDomain          string       `gorm:"type:varchar(255);index" json:"reseller_domain,omitempty"`                 // 下单分销域名快照
	ResellerProfitAmount    money.Amount `gorm:"type:decimal(20,2);not null;default:0" json:"reseller_profit_amount"`      // 分销差价快照
	ClientIP                string       `gorm:"type:varchar(64)" json:"client_ip,omitempty"`                              // 下单客户端IP
	RiskIP                  string       `gorm:"type:varchar(80);index;index:idx_orders_risk_pending,priority:1" json:"-"` // 规范化风控IP（IPv6按/64）
	ExpiresAt               *time.Time   `gorm:"index" json:"expires_at"`                                                  // 过期时间
	PaidAt                  *time.Time   `gorm:"index" json:"paid_at"`                                                     // 支付时间
	CanceledAt              *time.Time   `gorm:"index" json:"canceled_at"`                                                 // 取消时间
	CancelReason            string       `gorm:"type:varchar(32);not null;default:''" json:"cancel_reason,omitempty"`
	CreatedAt               time.Time    `gorm:"index" json:"created_at"`                                 // 创建时间
	UpdatedAt               time.Time    `gorm:"index" json:"updated_at"`                                 // 更新时间
	DeletedAt               *time.Time   `gorm:"index;index:idx_orders_risk_pending,priority:5" json:"-"` // 软删除时间

	Items []OrderItem `gorm:"foreignKey:OrderID" json:"items,omitempty"` // 订单项
	// 关联
	Fulfillment *fulfillmentdomain.Fulfillment `gorm:"foreignKey:OrderID" json:"fulfillment,omitempty"` // 交付记录
	Children    []Order                        `gorm:"foreignKey:ParentID" json:"children,omitempty"`   // 子订单
}

// TableName 指定表名
func (Order) TableName() string {
	return "orders"
}

// StripCostPrice 清除订单项中的成本价信息，避免前台用户看到成本价。
func (o *Order) StripCostPrice() {
	for i := range o.Items {
		o.Items[i].CostPrice = money.Amount{}
	}
	for i := range o.Children {
		o.Children[i].StripCostPrice()
	}
}

// MaskUpstreamFulfillmentType 将订单及子订单中的 upstream 交付类型替换为 manual，
// 避免前台用户感知到上游对接的存在。
func (o *Order) MaskUpstreamFulfillmentType() {
	const upstream = "upstream"
	const manual = "manual"
	for i := range o.Items {
		if o.Items[i].FulfillmentType == upstream {
			o.Items[i].FulfillmentType = manual
		}
	}
	if o.Fulfillment != nil && o.Fulfillment.Type == upstream {
		o.Fulfillment.Type = manual
	}
	for i := range o.Children {
		o.Children[i].MaskUpstreamFulfillmentType()
	}
}

// TruncateFulfillmentPayload 截断订单及子订单中超长的交付内容，防止前端渲染崩溃。
func (o *Order) TruncateFulfillmentPayload() {
	if o.Fulfillment != nil {
		o.Fulfillment.TruncatePayload(fulfillmentdomain.PayloadMaxPreviewLines)
	}
	for i := range o.Children {
		o.Children[i].TruncateFulfillmentPayload()
	}
}

// 开票类型：普票与专票互斥，空值表示不开票。
const (
	InvoiceTypeNone    = ""
	InvoiceTypeGeneral = "general" // 普票，税率 3%
	InvoiceTypeSpecial = "special" // 专票，税率 5%
)

var invoiceTaxRates = map[string]decimal.Decimal{
	InvoiceTypeGeneral: decimal.RequireFromString("0.03"),
	InvoiceTypeSpecial: decimal.RequireFromString("0.05"),
}

// NormalizeInvoiceType 归一化开票类型，未知值按不开票处理。
func NormalizeInvoiceType(invoiceType string) string {
	switch strings.ToLower(strings.TrimSpace(invoiceType)) {
	case InvoiceTypeGeneral:
		return InvoiceTypeGeneral
	case InvoiceTypeSpecial:
		return InvoiceTypeSpecial
	default:
		return InvoiceTypeNone
	}
}

// ResolveInvoiceType 兼容旧的布尔开票开关：未指定类型但要求开票时按普票处理。
func ResolveInvoiceType(invoiceType string, invoiceRequested bool) string {
	if normalized := NormalizeInvoiceType(invoiceType); normalized != InvoiceTypeNone {
		return normalized
	}
	if invoiceRequested {
		return InvoiceTypeGeneral
	}
	return InvoiceTypeNone
}

// InvoiceTaxRate 返回开票类型对应的税率，不开票时为 0。
func InvoiceTaxRate(invoiceType string) decimal.Decimal {
	if rate, ok := invoiceTaxRates[NormalizeInvoiceType(invoiceType)]; ok {
		return rate
	}
	return decimal.Zero
}

// ResolvedInvoiceType 返回订单实际生效的开票类型（兼容历史订单只有布尔标记的情况）。
func (o *Order) ResolvedInvoiceType() string {
	if o == nil {
		return InvoiceTypeNone
	}
	if !o.InvoiceRequested && NormalizeInvoiceType(o.InvoiceType) == InvoiceTypeNone {
		return InvoiceTypeNone
	}
	return ResolveInvoiceType(o.InvoiceType, o.InvoiceRequested)
}

// InvoiceTaxRate 返回订单生效的开票税率。
func (o *Order) InvoiceTaxRate() decimal.Decimal {
	return InvoiceTaxRate(o.ResolvedInvoiceType())
}
