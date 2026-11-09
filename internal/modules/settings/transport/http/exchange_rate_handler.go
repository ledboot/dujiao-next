package settingshttp

import (
	"errors"
	settingsapp "github.com/dujiao-next/internal/modules/settings/application/exchangerate"
	settingscontract "github.com/dujiao-next/internal/modules/settings/contract"
	"github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/gin-gonic/gin"
)

type ExchangeRateHandler struct {
	service *settingsapp.ExchangeRateService
}

func NewExchangeRateHandler(service *settingsapp.ExchangeRateService) *ExchangeRateHandler {
	return &ExchangeRateHandler{service: service}
}
func (h *ExchangeRateHandler) Get(c *gin.Context) {
	cfg, err := h.service.GetConfig()
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.settings_fetch_failed", err)
		return
	}
	snapshot, rateErr := h.service.GetExchangeRate(c.Request.Context(), false)
	// Keep configuration accessible when every upstream source is unavailable.
	response.Success(c, gin.H{"config": cfg, "snapshot": snapshot, "available": rateErr == nil})
}
func (h *ExchangeRateHandler) Update(c *gin.Context) {
	var cfg settingscontract.ExchangeRateConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		ginutil.RespondBindError(c, err)
		return
	}
	cfg, err := h.service.UpdateConfig(cfg)
	if err != nil {
		if errors.Is(err, settingscontract.ErrExchangeRateInvalid) {
			ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		} else {
			ginutil.RespondError(c, response.CodeInternal, "error.settings_save_failed", err)
		}
		return
	}
	response.Success(c, cfg)
}
func (h *ExchangeRateHandler) Refresh(c *gin.Context) {
	snapshot, err := h.service.GetExchangeRate(c.Request.Context(), true)
	if err != nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.exchange_rate_unavailable", nil)
		return
	}
	response.Success(c, snapshot)
}
func RegisterAdminExchangeRateRoutes(admin gin.IRoutes, handler *ExchangeRateHandler) {
	admin.GET("/settings/exchange-rate", handler.Get)
	admin.PUT("/settings/exchange-rate", handler.Update)
	admin.POST("/settings/exchange-rate/refresh", handler.Refresh)
}
