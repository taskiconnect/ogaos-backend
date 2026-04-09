package routes

import (
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	handlerAdminAuth "ogaos-backend/internal/api/handlers/admin_auth"
	handlerAuth "ogaos-backend/internal/api/handlers/auth"
	handlerBusiness "ogaos-backend/internal/api/handlers/business"
	handlerCoupon "ogaos-backend/internal/api/handlers/coupon"
	handlerCustomer "ogaos-backend/internal/api/handlers/customer"
	handlerDebt "ogaos-backend/internal/api/handlers/debt"
	handlerDigital "ogaos-backend/internal/api/handlers/digital"
	handlerExpense "ogaos-backend/internal/api/handlers/expense"
	handlerHealth "ogaos-backend/internal/api/handlers/health"
	handlerInvoice "ogaos-backend/internal/api/handlers/invoice"
	handlerLocation "ogaos-backend/internal/api/handlers/location"
	handlerProduct "ogaos-backend/internal/api/handlers/product"
	handlerRecruitment "ogaos-backend/internal/api/handlers/recruitment"
	handlerSale "ogaos-backend/internal/api/handlers/sale"
	handlerStore "ogaos-backend/internal/api/handlers/store"
	handlerSubscription "ogaos-backend/internal/api/handlers/subscription"
	handlerWebhook "ogaos-backend/internal/api/handlers/webhook"
	"ogaos-backend/internal/api/middleware"
	"ogaos-backend/internal/config"
)

func SetupAuthRoutes(
	r *gin.Engine,
	cfg *config.Config,
	db *gorm.DB,
	redisClient *goredis.Client,
	authHandler *handlerAuth.Handler,
	healthHandler *handlerHealth.Handler,
	locationHandler *handlerLocation.Handler,
	businessHandler *handlerBusiness.Handler,
	customerHandler *handlerCustomer.Handler,
	productHandler *handlerProduct.Handler,
	saleHandler *handlerSale.Handler,
	invoiceHandler *handlerInvoice.Handler,
	expenseHandler *handlerExpense.Handler,
	debtHandler *handlerDebt.Handler,
	storeHandler *handlerStore.Handler,
	recruitmentHandler *handlerRecruitment.Handler,
	digitalHandler *handlerDigital.Handler,
	webhookHandler *handlerWebhook.Handler,
	couponHandler *handlerCoupon.Handler,
	subscriptionHandler *handlerSubscription.Handler,
	adminAuthHandler *handlerAdminAuth.Handler,
) {
	userSecret := []byte(cfg.JWTSecret)
	adminSecret := []byte(cfg.AdminJWTSecret)

	r.Use(middleware.CORSMiddleware(cfg.AllowedOrigins))
	r.GET("/health", healthHandler.Check)

	webhooks := r.Group("/webhooks")
	{
		webhooks.POST("/paystack", webhookHandler.Paystack)
		webhooks.POST("/flutterwave", webhookHandler.Flutterwave)
	}

	v1 := r.Group("/api/v1")

	authGroup := v1.Group("/auth")
	{
		authGroup.POST("/register",
			middleware.UserRegisterRateLimiter(redisClient),
			authHandler.Register,
		)
		authGroup.POST("/login",
			middleware.UserLoginRateLimiter(redisClient),
			authHandler.Login,
		)
		authGroup.POST("/refresh", authHandler.Refresh)
		authGroup.POST("/logout", authHandler.Logout)
		authGroup.POST("/verify",
			middleware.UserVerifyEmailRateLimiter(redisClient),
			authHandler.VerifyEmail,
		)
		authGroup.POST("/resend-verification",
			middleware.UserResendVerificationRateLimiter(redisClient),
			authHandler.ResendVerification,
		)
	}

	locGroup := v1.Group("/locations")
	{
		locGroup.GET("/states", locationHandler.GetStates)
		locGroup.GET("/lgas", locationHandler.GetLGAs)
	}

	public := v1.Group("/public")
	{
		public.GET("/business/:slug", businessHandler.GetPublicProfile)
		public.GET("/business/:slug/keywords", businessHandler.GetPublicKeywords)
		public.GET("/keywords/suggestions", businessHandler.SuggestPublicKeywords)
		public.GET("/business/:slug/products", digitalHandler.ListPublicProducts)
		public.GET("/jobs/:slug", recruitmentHandler.GetPublicJob)
		public.POST("/jobs/:id/apply", recruitmentHandler.Apply)
		public.POST("/assessment/:app_id/submit", recruitmentHandler.SubmitAssessment)
		public.GET("/store/:slug", digitalHandler.GetPublicProduct)
		public.POST("/store/:id/purchase", digitalHandler.Purchase)
		public.GET("/orders/:order_id/download", digitalHandler.GetDownload)
	}

	protected := v1.Group("")
	protected.Use(middleware.AuthMiddleware(userSecret))

	protected.GET("/auth/me", authHandler.WhoAmI)

	biz := protected.Group("/business")
	biz.Use(middleware.RequireRole(middleware.RoleOwner))
	{
		biz.GET("/me", businessHandler.Get)
		biz.PATCH("/me", businessHandler.Update)
		biz.POST("/me/logo", businessHandler.UploadLogo)
		biz.PATCH("/me/visibility", businessHandler.SetVisibility)
		biz.POST("/me/gallery", businessHandler.AddGalleryImage)
		biz.DELETE("/me/gallery/:index", businessHandler.RemoveGalleryImage)
		biz.PATCH("/me/storefront-video", businessHandler.SetStorefrontVideo)
		biz.GET("/me/keywords", businessHandler.GetKeywords)
		biz.PUT("/me/keywords", businessHandler.SetKeywords)
	}

	staffGroup := protected.Group("/staff")
	staffGroup.Use(middleware.RequireRole(middleware.RoleOwner))
	{
		staffGroup.POST("",
			middleware.SubscriptionGuard(db, "staff_management"),
			middleware.LimitGuard(db, "staff"),
			authHandler.CreateStaff,
		)
		staffGroup.GET("", authHandler.ListStaff)
		staffGroup.DELETE("/:id", authHandler.DeactivateStaff)
	}

	stores := protected.Group("/stores")
	stores.Use(middleware.SubscriptionGuard(db, "stores"))
	{
		stores.POST("", middleware.RequireRole(middleware.RoleOwner), middleware.LimitGuard(db, "stores"), storeHandler.Create)
		stores.GET("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), storeHandler.List)
		stores.GET("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), storeHandler.Get)
		stores.PATCH("/:id", middleware.RequireRole(middleware.RoleOwner), storeHandler.Update)
		stores.PATCH("/:id/default", middleware.RequireRole(middleware.RoleOwner), storeHandler.SetDefault)
		stores.DELETE("/:id", middleware.RequireRole(middleware.RoleOwner), storeHandler.Delete)
	}

	customers := protected.Group("/customers")
	{
		customers.POST("",
			middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff),
			middleware.SubscriptionGuard(db, "customers_basic"),
			middleware.LimitGuard(db, "customers"),
			customerHandler.Create,
		)
		customers.GET("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), customerHandler.List)
		customers.GET("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), customerHandler.Get)
		customers.PATCH("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), customerHandler.Update)
		customers.DELETE("/:id", middleware.RequireRole(middleware.RoleOwner), customerHandler.Delete)
	}

	products := protected.Group("/products")
	{
		products.POST("",
			middleware.RequireRole(middleware.RoleOwner),
			middleware.SubscriptionGuard(db, "products"),
			middleware.LimitGuard(db, "products"),
			productHandler.Create,
		)
		products.GET("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), productHandler.List)
		products.GET("/scan", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), productHandler.Scan)
		products.GET("/inventory", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), productHandler.Inventory)
		products.GET("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), productHandler.Get)
		products.PATCH("/:id", middleware.RequireRole(middleware.RoleOwner), productHandler.Update)
		products.DELETE("/:id", middleware.RequireRole(middleware.RoleOwner), productHandler.Delete)
		products.POST("/:id/stock", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), productHandler.AdjustStock)
		products.POST("/:id/image", middleware.RequireRole(middleware.RoleOwner), productHandler.UploadImage)
	}

	sales := protected.Group("/sales")
	{
		sales.POST("",
			middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff),
			middleware.SubscriptionGuard(db, "sales"),
			middleware.LimitGuard(db, "sales"),
			saleHandler.Create,
		)
		sales.GET("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), saleHandler.List)
		sales.GET("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), saleHandler.Get)
		sales.POST("/:id/receipt", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), saleHandler.GenerateReceipt)
		sales.POST("/:id/payment", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), saleHandler.RecordPayment)
		sales.PATCH("/:id/cancel", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), saleHandler.Cancel)
	}

	invoices := protected.Group("/invoices")
	invoices.Use(middleware.SubscriptionGuard(db, "invoices"))
	{
		invoices.POST("", middleware.RequireRole(middleware.RoleOwner), invoiceHandler.Create)
		invoices.GET("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), invoiceHandler.List)
		invoices.GET("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), invoiceHandler.Get)
		invoices.POST("/:id/send", middleware.RequireRole(middleware.RoleOwner), invoiceHandler.Send)
		invoices.POST("/:id/payment", middleware.RequireRole(middleware.RoleOwner), invoiceHandler.RecordPayment)
		invoices.DELETE("/:id", middleware.RequireRole(middleware.RoleOwner), invoiceHandler.Cancel)
	}

	expenses := protected.Group("/expenses")
	expenses.Use(middleware.SubscriptionGuard(db, "expense_tracking"))
	{
		expenses.GET("/summary", middleware.RequireRole(middleware.RoleOwner), expenseHandler.MonthlySummary)
		expenses.POST("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), expenseHandler.Create)
		expenses.GET("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), expenseHandler.List)
		expenses.GET("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), expenseHandler.Get)
		expenses.PATCH("/:id", middleware.RequireRole(middleware.RoleOwner), expenseHandler.Update)
		expenses.DELETE("/:id", middleware.RequireRole(middleware.RoleOwner), expenseHandler.Delete)
	}

	debts := protected.Group("/debts")
	debts.Use(middleware.SubscriptionGuard(db, "debt_tracking"))
	{
		debts.POST("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), debtHandler.Create)
		debts.GET("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), debtHandler.List)
		debts.GET("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), debtHandler.Get)
		debts.POST("/:id/payment", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), debtHandler.RecordPayment)
	}

	recruit := protected.Group("")
	recruit.Use(middleware.SubscriptionGuard(db, "recruitment"))
	{
		jobs := recruit.Group("/jobs")
		jobs.POST("", middleware.RequireRole(middleware.RoleOwner), recruitmentHandler.CreateJob)
		jobs.GET("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), recruitmentHandler.ListJobs)
		jobs.GET("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), recruitmentHandler.GetJob)
		jobs.PATCH("/:id/close", middleware.RequireRole(middleware.RoleOwner), recruitmentHandler.CloseJob)

		apps := recruit.Group("/applications")
		apps.GET("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), recruitmentHandler.ListApplications)
		apps.PATCH("/:id/review", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), recruitmentHandler.ReviewApplication)
	}

	dp := protected.Group("/digital-products")
	{
		dp.POST("", middleware.RequireRole(middleware.RoleOwner), digitalHandler.Create)
		dp.GET("", middleware.RequireRole(middleware.RoleOwner), digitalHandler.List)
		dp.GET("/:id", middleware.RequireRole(middleware.RoleOwner), digitalHandler.Get)
		dp.PATCH("/:id", middleware.RequireRole(middleware.RoleOwner), digitalHandler.Update)
		dp.DELETE("/:id", middleware.RequireRole(middleware.RoleOwner), digitalHandler.Delete)
		dp.POST("/:id/file", middleware.RequireRole(middleware.RoleOwner), digitalHandler.UploadFile)
		dp.POST("/:id/cover", middleware.RequireRole(middleware.RoleOwner), digitalHandler.UploadCover)
		dp.POST("/:id/gallery", middleware.RequireRole(middleware.RoleOwner), digitalHandler.AddGalleryImage)
		dp.DELETE("/:id/gallery/:index", middleware.RequireRole(middleware.RoleOwner), digitalHandler.RemoveGalleryImage)
	}

	subs := protected.Group("/subscriptions")
	{
		subs.GET("/me", subscriptionHandler.Get)
		subs.POST("/validate-coupon", subscriptionHandler.ValidateCoupon)
		subs.POST("/initiate", subscriptionHandler.Initiate)
		subs.POST("/verify", subscriptionHandler.Verify)
	}

	adminAuthGroup := v1.Group("/admin/auth")
	{
		adminAuthGroup.POST("/login",
			middleware.AdminLoginRateLimiter(redisClient),
			adminAuthHandler.Login,
		)
		adminAuthGroup.POST("/verify-otp",
			middleware.AdminVerifyOTPRateLimiter(redisClient),
			adminAuthHandler.VerifyOTP,
		)
		adminAuthGroup.POST("/resend-otp",
			middleware.AdminResendOTPRateLimiter(redisClient),
			adminAuthHandler.ResendOTP,
		)
		adminAuthGroup.POST("/setup-password", adminAuthHandler.SetupPassword)
		adminAuthGroup.POST("/refresh", adminAuthHandler.Refresh)
		adminAuthGroup.POST("/logout", adminAuthHandler.Logout)
	}

	admin := v1.Group("/admin")
	admin.Use(middleware.AdminAuthMiddleware(adminSecret))
	{
		admin.GET("/me", adminAuthHandler.WhoAmI)
		admin.GET("/dashboard", func(c *gin.Context) {
			c.JSON(200, gin.H{"success": true, "message": "welcome to the admin dashboard"})
		})

		analytics := admin.Group("/analytics")
		{
			analytics.GET("/overview",
				middleware.RequireAdminRole(
					middleware.AdminRoleSuperAdmin,
					middleware.AdminRoleSupport,
					middleware.AdminRoleFinance,
				),
				placeholderOK,
			)
			analytics.GET("/businesses",
				middleware.RequireAdminRole(
					middleware.AdminRoleSuperAdmin,
					middleware.AdminRoleSupport,
				),
				placeholderOK,
			)
			analytics.GET("/revenue",
				middleware.RequireAdminRole(
					middleware.AdminRoleSuperAdmin,
					middleware.AdminRoleFinance,
				),
				placeholderOK,
			)
		}

		admins := admin.Group("/admins")
		admins.Use(middleware.RequireAdminRole(middleware.AdminRoleSuperAdmin))
		{
			admins.POST("/invite", adminAuthHandler.InviteAdmin)
			admins.GET("", adminAuthHandler.ListAdmins)
			admins.GET("/:id", adminAuthHandler.GetAdmin)
			admins.PATCH("/:id", adminAuthHandler.UpdateAdmin)
			admins.DELETE("/:id", adminAuthHandler.DeactivateAdmin)
		}

		settings := admin.Group("/settings")
		settings.Use(middleware.RequireAdminRole(middleware.AdminRoleSuperAdmin))
		{
			settings.GET("", placeholderOK)
			settings.PATCH("", placeholderOK)
		}

		coupons := admin.Group("/coupons")
		{
			coupons.POST("",
				middleware.RequireAdminRole(middleware.AdminRoleSuperAdmin),
				couponHandler.Create,
			)
			coupons.GET("",
				middleware.RequireAdminRole(middleware.AdminRoleSuperAdmin, middleware.AdminRoleSupport),
				couponHandler.List,
			)
			coupons.GET("/:id",
				middleware.RequireAdminRole(middleware.AdminRoleSuperAdmin, middleware.AdminRoleSupport),
				couponHandler.Get,
			)
			coupons.PATCH("/:id",
				middleware.RequireAdminRole(middleware.AdminRoleSuperAdmin),
				couponHandler.Update,
			)
			coupons.DELETE("/:id",
				middleware.RequireAdminRole(middleware.AdminRoleSuperAdmin),
				couponHandler.Delete,
			)
		}
	}
}

func placeholderOK(c *gin.Context) {
	c.JSON(200, gin.H{"success": true, "data": nil, "message": "not yet implemented"})
}
