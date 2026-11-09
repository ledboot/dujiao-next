package adminhttp

import (
	"context"
	"errors"
	settingscontract "github.com/dujiao-next/internal/modules/settings/contract"
	"github.com/shopspring/decimal"
	"strings"
	"time"

	resellerdomain "github.com/dujiao-next/internal/modules/reseller/domain"

	productcontract "github.com/dujiao-next/internal/modules/catalog/product/contract"

	resellermodule "github.com/dujiao-next/internal/modules/reseller/contract"
	"github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"

	"github.com/gin-gonic/gin"
)

// AdminFinanceQueryService 是后台分销财务查询端点所需的最小用例接口。
type AdminFinanceQueryService interface {
	ListAdminLedgerEntries(filter resellermodule.AdminLedgerListFilter) ([]resellerdomain.LedgerEntry, int64, error)
	ListAdminBalanceAccounts(filter resellermodule.AdminBalanceAccountListFilter) ([]resellerdomain.BalanceAccount, int64, error)
	ListAdminWithdrawRequests(filter resellermodule.AdminWithdrawListFilter) ([]resellerdomain.WithdrawRequest, int64, error)
}

// AdminWithdrawReviewService 是后台提现审核写操作所需的最小用例接口。
type AdminWithdrawReviewService interface {
	ReviewWithdraw(adminID, withdrawID uint, action, reason string) (*resellerdomain.WithdrawRequest, error)
}

// AdminFinanceHandler 处理后台分销财务请求。
type AdminFinanceHandler struct {
	query    AdminFinanceQueryService
	reviewer AdminWithdrawReviewService
}

func NewAdminFinanceHandler(query AdminFinanceQueryService, reviewer AdminWithdrawReviewService) *AdminFinanceHandler {
	if query == nil {
		panic("reseller admin finance handler: query is nil")
	}
	if reviewer == nil {
		panic("reseller admin finance handler: reviewer is nil")
	}
	return &AdminFinanceHandler{query: query, reviewer: reviewer}
}

type reviewWithdrawRequest struct {
	Reason string `json:"reason"`
}

// ListLedgerEntries 管理端分销账务流水列表。
func (h *AdminFinanceHandler) ListLedgerEntries(c *gin.Context) {
	page, pageSize := ginutil.ParsePagination(c)
	resellerID, _ := ginutil.ParseQueryUint(c.Query("reseller_id"), false)
	userID, _ := ginutil.ParseQueryUint(c.Query("user_id"), false)
	orderID, _ := ginutil.ParseQueryUint(c.Query("order_id"), false)
	rows, total, err := h.query.ListAdminLedgerEntries(resellermodule.AdminLedgerListFilter{
		Page:        page,
		PageSize:    pageSize,
		ResellerID:  resellerID,
		UserID:      userID,
		Keyword:     strings.TrimSpace(c.Query("keyword")),
		Type:        strings.TrimSpace(c.Query("type")),
		Status:      strings.TrimSpace(c.Query("status")),
		OrderID:     orderID,
		OrderNo:     strings.TrimSpace(c.Query("order_no")),
		CreatedFrom: parseFinanceTimePointer(c.Query("created_from")),
		CreatedTo:   parseFinanceTimePointer(c.Query("created_to")),
	})
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.SuccessWithPage(c, rows, response.BuildPagination(page, pageSize, total))
}

// ListBalanceAccounts 管理端分销余额账户列表。
func (h *AdminFinanceHandler) ListBalanceAccounts(c *gin.Context) {
	page, pageSize := ginutil.ParsePagination(c)
	resellerID, _ := ginutil.ParseQueryUint(c.Query("reseller_id"), false)
	userID, _ := ginutil.ParseQueryUint(c.Query("user_id"), false)
	rows, total, err := h.query.ListAdminBalanceAccounts(resellermodule.AdminBalanceAccountListFilter{
		Page:       page,
		PageSize:   pageSize,
		ResellerID: resellerID,
		UserID:     userID,
		Keyword:    strings.TrimSpace(c.Query("keyword")),
		Status:     strings.TrimSpace(c.Query("status")),
	})
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.SuccessWithPage(c, rows, response.BuildPagination(page, pageSize, total))
}

// ListWithdraws 管理端分销提现申请列表。
func (h *AdminFinanceHandler) ListWithdraws(c *gin.Context) {
	page, pageSize := ginutil.ParsePagination(c)
	resellerID, _ := ginutil.ParseQueryUint(c.Query("reseller_id"), false)
	userID, _ := ginutil.ParseQueryUint(c.Query("user_id"), false)
	rows, total, err := h.query.ListAdminWithdrawRequests(resellermodule.AdminWithdrawListFilter{
		Page:        page,
		PageSize:    pageSize,
		ResellerID:  resellerID,
		UserID:      userID,
		Keyword:     strings.TrimSpace(c.Query("keyword")),
		Status:      strings.TrimSpace(c.Query("status")),
		CreatedFrom: parseFinanceTimePointer(c.Query("created_from")),
		CreatedTo:   parseFinanceTimePointer(c.Query("created_to")),
	})
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.user_fetch_failed", err)
		return
	}
	response.SuccessWithPage(c, rows, response.BuildPagination(page, pageSize, total))
}

// RejectWithdraw 拒绝分销提现申请。
func (h *AdminFinanceHandler) RejectWithdraw(c *gin.Context) {
	adminID, ok := ginutil.GetAdminID(c)
	if !ok {
		return
	}
	id, err := ginutil.ParseParamUint(c, "id")
	if err != nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	var req reviewWithdrawRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ginutil.RespondBindError(c, err)
		return
	}
	row, err := h.reviewer.ReviewWithdraw(adminID, id, "reject", req.Reason)
	if err != nil {
		respondAdminWithdrawReviewError(c, err)
		return
	}
	response.Success(c, row)
}

// PayWithdraw 标记分销提现已打款。
func (h *AdminFinanceHandler) PayWithdraw(c *gin.Context) {
	adminID, ok := ginutil.GetAdminID(c)
	if !ok {
		return
	}
	id, err := ginutil.ParseParamUint(c, "id")
	if err != nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	var row *resellerdomain.WithdrawRequest
	if reviewer, ok := h.reviewer.(interface {
		ReviewWithdrawAtRate(uint, uint, decimal.Decimal) (*resellerdomain.WithdrawRequest, error)
	}); ok {
		var req struct {
			PayoutRate decimal.Decimal `json:"payout_rate"`
		}
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&req); err != nil {
				ginutil.RespondBindError(c, err)
				return
			}
		}
		row, err = reviewer.ReviewWithdrawAtRate(adminID, id, req.PayoutRate)
	} else {
		row, err = h.reviewer.ReviewWithdraw(adminID, id, "pay", "")
	}
	if err != nil {
		respondAdminWithdrawReviewError(c, err)
		return
	}
	response.Success(c, row)
}

func respondAdminWithdrawReviewError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, productcontract.ErrNotFound):
		ginutil.RespondError(c, response.CodeNotFound, "error.bad_request", nil)
	case errors.Is(err, settingscontract.ErrExchangeRateUnavailable):
		ginutil.RespondError(c, response.CodeBadRequest, "error.exchange_rate_unavailable", nil)
	case errors.Is(err, resellermodule.ErrWithdrawCurrencyUnavailable):
		ginutil.RespondError(c, response.CodeBadRequest, "error.reseller_withdraw_currency_unavailable", nil)
	case errors.Is(err, resellermodule.ErrWithdrawAddressInvalid):
		ginutil.RespondError(c, response.CodeBadRequest, "error.reseller_withdraw_address_invalid", nil)
	case errors.Is(err, resellermodule.ErrWithdrawBelowMinimum):
		ginutil.RespondError(c, response.CodeBadRequest, "error.reseller_withdraw_minimum", nil)
	case errors.Is(err, resellermodule.ErrWithdrawAmountInvalid):
		ginutil.RespondError(c, response.CodeBadRequest, "error.reseller_withdraw_amount_invalid", nil)
	case errors.Is(err, resellermodule.ErrWithdrawStatusInvalid):
		ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
	default:
		ginutil.RespondError(c, response.CodeInternal, "error.save_failed", err)
	}
}

func parseFinanceTimePointer(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t
	}
	return nil
}

func (h *AdminFinanceHandler) GetPayoutQuote(c *gin.Context) {
	id, err := ginutil.ParseParamUint(c, "id")
	if err != nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	provider, ok := h.reviewer.(interface {
		GetWithdrawPayoutQuote(context.Context, uint) (resellermodule.WithdrawPayoutQuote, error)
	})
	if !ok {
		ginutil.RespondError(c, response.CodeBadRequest, "error.exchange_rate_unavailable", nil)
		return
	}
	quote, err := provider.GetWithdrawPayoutQuote(c.Request.Context(), id)
	if err != nil {
		respondAdminWithdrawReviewError(c, err)
		return
	}
	response.Success(c, quote)
}
