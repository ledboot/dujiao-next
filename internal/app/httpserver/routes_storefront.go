package httpserver

import (
	"net/http"
	"strings"

	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/app/httpserver/middleware"
	affiliatebootstrap "github.com/dujiao-next/internal/bootstrap/affiliate"
	"github.com/dujiao-next/internal/config"
	affiliatetransport "github.com/dujiao-next/internal/modules/affiliate/transport/http"
	apicredentialtransport "github.com/dujiao-next/internal/modules/apicredential/transport/http"
	auditlogtransport "github.com/dujiao-next/internal/modules/auditlog/transport/http"
	captchatransport "github.com/dujiao-next/internal/modules/captcha/transport/http"
	carttransport "github.com/dujiao-next/internal/modules/cart/transport/http"
	categoryhttp "github.com/dujiao-next/internal/modules/catalog/category/transport/http"
	producthttp "github.com/dujiao-next/internal/modules/catalog/product/transport/http"
	contenttransport "github.com/dujiao-next/internal/modules/content/transport/http"
	giftcardtransport "github.com/dujiao-next/internal/modules/giftcard/transport/http"
	userauthtransport "github.com/dujiao-next/internal/modules/identity/userauth/transport/http"
	memberleveltransport "github.com/dujiao-next/internal/modules/memberlevel/transport/http"
	ordertransport "github.com/dujiao-next/internal/modules/order/transport/http"
	paymenttransport "github.com/dujiao-next/internal/modules/payment/transport/http"
	paymentcallbacktransport "github.com/dujiao-next/internal/modules/payment/transport/http/callback"
	resellercontract "github.com/dujiao-next/internal/modules/reseller/contract"
	resellertransport "github.com/dujiao-next/internal/modules/reseller/transport/http/user"
	publicconfigtransport "github.com/dujiao-next/internal/modules/settings/transport/http/public"
	wallettransport "github.com/dujiao-next/internal/modules/wallet/transport/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func registerStorefrontRoutes(
	apiV1 *gin.RouterGroup,
	cfg *config.Config,
	c *container.Container,
	publicContentHandler *contenttransport.PublicHandler,
	publicCatalogHandler *producthttp.PublicHandler,
	publicCategoryHandler *categoryhttp.PublicHandler,
	userResellerHandler *resellertransport.UserHandler,
	userResellerProductSettingHandler *resellertransport.UserProductSettingHandler,
	userResellerFinanceHandler *resellertransport.UserFinanceHandler,
	userResellerOrderHandler *resellertransport.UserOrderHandler,
	userApiCredentialHandler *apicredentialtransport.UserHandler,
	userAuditLogHandler *auditlogtransport.UserHandler,
	userGiftCardHandler *giftcardtransport.UserHandler,
	publicMemberLevelHandler *memberleveltransport.PublicHandler,
	userProfileHandler *userauthtransport.UserProfileHandler,
	userEmailHandler *userauthtransport.UserEmailHandler,
	userPasswordHandler *userauthtransport.UserPasswordHandler,
	userVerifyHandler *userauthtransport.UserVerifyHandler,
	userTelegramOIDCHandler *userauthtransport.UserTelegramOIDCHandler,
	userTelegramHandler *userauthtransport.UserTelegramHandler,
	userGoogleHandler *userauthtransport.UserGoogleHandler,
	userLoginHandler *userauthtransport.UserLoginHandler,
	user2FAHandler *userauthtransport.User2FAHandler,
	userInviteHandler *userauthtransport.UserInviteHandler,
	publicConfigHandler *publicconfigtransport.Handler,
	userCartHandler *carttransport.UserHandler,
	userOrderHandler *ordertransport.UserHandler,
	guestOrderHandler *ordertransport.GuestHandler,
	orderPreviewHandler *ordertransport.PreviewHandler,
	orderCreateHandler *ordertransport.CreateHandler,
	paymentLatestHandler *paymenttransport.LatestHandler,
	paymentWriteHandler *paymenttransport.WriteHandler,
	userWalletHandler *wallettransport.UserHandler,
	redisClient *redis.Client,
	loginRule middleware.RateLimitRule,
	guestReadRule middleware.RateLimitRule,
	guestWriteRule middleware.RateLimitRule,
	giftCardRedeemRule middleware.RateLimitRule,
) {
	storefront := apiV1.Group("")
	storefront.Use(middleware.ResellerTenantMiddleware(c.ResellerDomainResolver))
	affiliateHandler := affiliatebootstrap.NewStorefrontHandler(c)
	gate := captchatransport.NewStorefrontGate(c.CaptchaService, cfg.UserJWT.SecretKey, func(r *http.Request) bool {
		tenant, _ := resellercontract.TenantFromContext(r.Context())
		return tenant.ResellerID == nil
	})
	verifyRule := guestWriteRule
	verifyRule.Prefix += ":storefront-captcha"
	verifyLimit := middleware.RateLimitMiddleware(redisClient, verifyRule, middleware.KeyByIP)

	// 公开接口
	bootstrap := storefront.Group("/public")
	publicReadRule := guestReadRule
	publicReadRule.Prefix += ":public_read"
	public := storefront.Group("/public", publicReadingGate(gate.Require),
		middleware.RateLimitMiddleware(redisClient, publicReadRule, func(ctx *gin.Context) string {
			tenant := resellercontract.TenantOrMain(ctx.Request.Context())
			return tenant.Host + ":" + ctx.ClientIP()
		}))
	{
		publicconfigtransport.RegisterPublicRoutes(bootstrap, publicConfigHandler)
		producthttp.RegisterPublicRoutes(public, publicCatalogHandler)
		categoryhttp.RegisterPublicRoutes(public, publicCategoryHandler)
		contenttransport.RegisterPublicRoutes(public, publicContentHandler)
		captchatransport.RegisterPublicRoutes(bootstrap, captchatransport.NewPublicHandler(c.CaptchaService))
		gate.RegisterRoutes(bootstrap, verifyLimit)
		affiliatetransport.RegisterPublicRoutes(bootstrap, affiliateHandler)
		memberleveltransport.RegisterPublicRoutes(public, publicMemberLevelHandler)
	}

	// 游客接口
	guest := storefront.Group("/guest", gate.Require)
	guestRead := guest.Group("")
	guestRead.Use(middleware.RateLimitMiddleware(redisClient, guestReadRule, middleware.KeyByIP))
	{
		ordertransport.RegisterGuestPreviewRoute(guestRead, orderPreviewHandler)
		ordertransport.RegisterGuestReadRoutes(guestRead, guestOrderHandler)
		paymenttransport.RegisterGuestLatestRoute(guestRead, paymentLatestHandler)
	}
	guestWrite := guest.Group("")
	guestWrite.Use(middleware.RateLimitMiddleware(redisClient, guestWriteRule, middleware.KeyByIP))
	{
		ordertransport.RegisterGuestCreateRoute(guestWrite, orderCreateHandler)
		ordertransport.RegisterGuestCreateAndPayRoute(guestWrite, orderCreateHandler)
		paymenttransport.RegisterGuestWriteRoutes(guestWrite, paymentWriteHandler)
	}

	// 注册 / 忘记密码 / 发送验证码：按 IP 计数，共享出口 IP（NAT）下一次完整流程就要 2~4 次请求，
	// 不能照搬登录的 5 次/封 15 分钟，否则同网用户会被成批误封。
	authRule := middleware.RateLimitRule{
		Prefix:        loginRule.Prefix + ":auth",
		WindowSeconds: 300,
		MaxRequests:   20,
		BlockSeconds:  300,
		MessageKey:    "error.rate_limited",
	}
	authRateLimit := middleware.RateLimitMiddleware(redisClient, authRule, middleware.KeyByIP)

	// 用户认证接口
	auth := storefront.Group("/auth")
	auth.Use(func(c *gin.Context) {
		// Google posts the redirect result cross-site, so it cannot send the clearance cookie.
		if c.FullPath() == auth.BasePath()+"/google/redirect/callback" {
			c.Next()
			return
		}
		gate.Require(c)
	})
	{
		userauthtransport.RegisterUserVerifyAuthRoutes(auth, userVerifyHandler, authRateLimit)
		userauthtransport.RegisterUserRegisterAuthRoutes(auth, userLoginHandler, authRateLimit)
		userauthtransport.RegisterUserLoginAuthRoutes(auth, userLoginHandler, middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByIPAndJSONField("email")))
		userauthtransport.RegisterUser2FAAuthRoutes(auth, user2FAHandler, middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByIP))
		userauthtransport.RegisterUserTelegramAuthRoutes(auth, userTelegramHandler, middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByIP))
		userauthtransport.RegisterUserTelegramOIDCAuthRoutes(auth, userTelegramOIDCHandler, middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByIP))
		userauthtransport.RegisterUserGoogleAuthRoutes(auth, userGoogleHandler, middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByIP))
		userauthtransport.RegisterUserInviteAuthRoutes(auth, userInviteHandler, middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByIP))
		userauthtransport.RegisterUserPasswordAuthRoutes(auth, userPasswordHandler, authRateLimit)
	}

	// 用户接口（需鉴权）
	user := storefront.Group("", gate.Require)
	user.Use(middleware.UserJWTAuthMiddleware(cfg.UserJWT.SecretKey, c.UserStore))
	{
		userauthtransport.RegisterUserProfileRoutes(user, userProfileHandler)
		auditlogtransport.RegisterUserRoutes(user, userAuditLogHandler)
		userauthtransport.RegisterUserPasswordRoutes(user, userPasswordHandler)
		userauthtransport.RegisterUserTelegramRoutes(user, userTelegramHandler)
		userauthtransport.RegisterUserTelegramOIDCRoutes(user, userTelegramOIDCHandler)
		userauthtransport.RegisterUserGoogleRoutes(
			user,
			userGoogleHandler,
			middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByUserIDAndIP),
		)
		userauthtransport.RegisterUserEmailRoutes(user, userEmailHandler)
		userauthtransport.RegisterUser2FARoutes(user, user2FAHandler)
		carttransport.RegisterUserRoutes(user, userCartHandler)
		ordertransport.RegisterUserCreateRoute(user, orderCreateHandler)
		ordertransport.RegisterUserCreateAndPayRoute(user, orderCreateHandler)
		ordertransport.RegisterUserPreviewRoute(user, orderPreviewHandler)
		ordertransport.RegisterUserPaymentChannelsRoute(user, userOrderHandler)
		ordertransport.RegisterUserReadRoutes(user, userOrderHandler)
		ordertransport.RegisterUserCancelRoute(user, userOrderHandler)
		paymenttransport.RegisterUserWriteRoutes(user, paymentWriteHandler)
		paymenttransport.RegisterUserLatestRoute(user, paymentLatestHandler)
		wallettransport.RegisterUserRoutes(user, userWalletHandler)
		giftCardRedeem := user.Group("", middleware.RateLimitMiddleware(redisClient, giftCardRedeemRule, middleware.KeyByUserIDAndIP))
		giftcardtransport.RegisterUserRoutes(giftCardRedeem, userGiftCardHandler)
		affiliatetransport.RegisterUserRoutes(user, affiliateHandler)

		resellerConsole := user.Group("/reseller")
		resellerConsole.Use(middleware.RequireMainTenantForResellerConsole())
		{
			resellertransport.RegisterUserConsoleRoutes(resellerConsole, userResellerHandler)
			resellertransport.RegisterUserProductSettingRoutes(resellerConsole, userResellerProductSettingHandler)
			resellertransport.RegisterUserFinanceRoutes(resellerConsole, userResellerFinanceHandler)
			resellertransport.RegisterUserOrderRoutes(resellerConsole, userResellerOrderHandler)
			announcementConsole := resellerConsole.Group("")
			announcementConsole.Use(userResellerHandler.RequireAnnouncementOwner)
			contenttransport.RegisterResellerAnnouncementRoutes(announcementConsole, contenttransport.NewAdminHandler(c.ContentPostService, nil, nil, nil))
		}

		// API 对接权限（用户中心）
		apicredentialtransport.RegisterUserRoutes(user, userApiCredentialHandler)
	}
}

// Public reading is an explicit GET allowlist. New endpoints and all writes
// keep the clearance requirement until deliberately classified as public.
func publicReadingGate(require gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet {
			switch c.FullPath() {
			case "/api/v1/public/products", "/api/v1/public/products/:slug",
				"/api/v1/public/categories", "/api/v1/public/posts", "/api/v1/public/posts/:slug",
				"/api/v1/public/banners", "/api/v1/public/post-categories", "/api/v1/public/member-levels":
				c.Next()
				return
			}
		}
		require(c)
	}
}

func publicReadingPath(path string) bool {
	switch path {
	case "/", "/products", "/blog", "/notice", "/about", "/terms", "/privacy", "/sitemap.xml", "/robots.txt":
		return true
	}
	return strings.HasPrefix(path, "/products/") || strings.HasPrefix(path, "/categories/") || strings.HasPrefix(path, "/blog/")
}

func registerPaymentCallbackRoutes(apiV1 *gin.RouterGroup, callbackHandler *paymentcallbacktransport.Handler, webhookHandler *paymenttransport.WebhookHandler, epayRedirectHandler *paymenttransport.EpayRedirectHandler, redisClient *redis.Client, callbackRule middleware.RateLimitRule) {
	callbacks := apiV1.Group("", middleware.RateLimitMiddleware(redisClient, callbackRule, middleware.KeyByIP))
	paymentcallbacktransport.RegisterRoutes(callbacks, callbackHandler)
	paymenttransport.RegisterWebhookRoutes(callbacks, webhookHandler)
	paymenttransport.RegisterEpayRedirectRoute(callbacks, epayRedirectHandler)
}
