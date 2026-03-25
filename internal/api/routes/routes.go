package routes

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ogaos-backend/internal/api/handlers/auth"
	"ogaos-backend/internal/api/handlers/business"
	"ogaos-backend/internal/api/handlers/coupon"
	"ogaos-backend/internal/api/handlers/customer"
	"ogaos-backend/internal/api/handlers/debt"
	"ogaos-backend/internal/api/handlers/digital"
	"ogaos-backend/internal/api/handlers/expense"
	"ogaos-backend/internal/api/handlers/health"
	"ogaos-backend/internal/api/handlers/invoice"
	"ogaos-backend/internal/api/handlers/location"
	"ogaos-backend/internal/api/handlers/product"
	"ogaos-backend/internal/api/handlers/recruitment"
	"ogaos-backend/internal/api/handlers/sale"
	"ogaos-backend/internal/api/handlers/store"
	"ogaos-backend/internal/api/handlers/subscription"
	"ogaos-backend/internal/api/handlers/webhook"
	"ogaos-backend/internal/api/middleware"
	"ogaos-backend/internal/config"
)

func SetupAuthRoutes(
	r *gin.Engine,
	cfg *config.Config,
	db *gorm.DB,
	authHandler *auth.Handler,
	healthHandler *health.Handler,
	locationHandler *location.Handler,
	businessHandler *business.Handler,
	customerHandler *customer.Handler,
	productHandler *product.Handler,
	saleHandler *sale.Handler,
	invoiceHandler *invoice.Handler,
	expenseHandler *expense.Handler,
	debtHandler *debt.Handler,
	storeHandler *store.Handler,
	recruitmentHandler *recruitment.Handler,
	digitalHandler *digital.Handler,
	webhookHandler *webhook.Handler,
	couponHandler *coupon.Handler,
	subscriptionHandler *subscription.Handler,
) {
	// ── Global ────────────────────────────────────────────────────────────────
	r.Use(middleware.CORSMiddleware(cfg.AllowedOrigins))

	// ── Health ────────────────────────────────────────────────────────────────
	r.GET("/health", healthHandler.Check)

	// ── Webhooks (raw body, no JWT) ───────────────────────────────────────────
	webhooks := r.Group("/webhooks")
	{
		webhooks.POST("/paystack", webhookHandler.Paystack)
		webhooks.POST("/flutterwave", webhookHandler.Flutterwave)
	}

	v1 := r.Group("/api/v1")

	// ── Auth (public) ─────────────────────────────────────────────────────────
	authGroup := v1.Group("/auth")
	{
		authGroup.POST("/register", authHandler.Register)
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/refresh", authHandler.Refresh)
		authGroup.POST("/logout", authHandler.Logout)
		authGroup.GET("/verify", authHandler.VerifyEmail)
	}

	// ── Location (public) ─────────────────────────────────────────────────────
	locGroup := v1.Group("/locations")
	{
		locGroup.GET("/states", locationHandler.GetStates)
		locGroup.GET("/lgas", locationHandler.GetLGAs)
	}

	// ── Public storefront & jobs (no JWT) ─────────────────────────────────────
	public := v1.Group("/public")
	{
		public.GET("/business/:slug", businessHandler.GetPublicProfile)
		public.GET("/business/:slug/keywords", businessHandler.GetPublicKeywords)
		public.GET("/business/:slug/products", digitalHandler.ListPublicProducts)

		public.GET("/jobs/:slug", recruitmentHandler.GetPublicJob)
		public.POST("/jobs/:id/apply", recruitmentHandler.Apply)
		public.POST("/assessment/:app_id/submit", recruitmentHandler.SubmitAssessment)

		public.GET("/store/:slug", digitalHandler.GetPublicProduct)
		public.POST("/store/:id/purchase", digitalHandler.Purchase)
		public.GET("/orders/:order_id/download", digitalHandler.GetDownload)
	}

	// ── Protected (JWT required) ──────────────────────────────────────────────
	protected := v1.Group("")
	protected.Use(middleware.AuthMiddleware([]byte(cfg.JWTSecret)))

	protected.GET("/auth/me", authHandler.WhoAmI)

	// ── Business ──────────────────────────────────────────────────────────────
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

	// ── Staff ─────────────────────────────────────────────────────────────────
	staff := protected.Group("/staff")
	staff.Use(middleware.RequireRole(middleware.RoleOwner))
	{
		staff.POST("", authHandler.CreateStaff)
		staff.DELETE("/:id", authHandler.DeactivateStaff)
	}

	// ── Stores ────────────────────────────────────────────────────────────────
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

	// ── Customers ─────────────────────────────────────────────────────────────
	customers := protected.Group("/customers")
	{
		customers.POST("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), middleware.LimitGuard(db, "customers"), customerHandler.Create)
		customers.GET("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), customerHandler.List)
		customers.GET("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), customerHandler.Get)
		customers.PATCH("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), customerHandler.Update)
		customers.DELETE("/:id", middleware.RequireRole(middleware.RoleOwner), customerHandler.Delete)
	}

	// ── Products ──────────────────────────────────────────────────────────────
	products := protected.Group("/products")
	{
		products.POST("", middleware.RequireRole(middleware.RoleOwner), middleware.LimitGuard(db, "products"), productHandler.Create)
		products.GET("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), productHandler.List)
		products.GET("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), productHandler.Get)
		products.PATCH("/:id", middleware.RequireRole(middleware.RoleOwner), productHandler.Update)
		products.DELETE("/:id", middleware.RequireRole(middleware.RoleOwner), productHandler.Delete)
		products.POST("/:id/stock", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), productHandler.AdjustStock)
		products.POST("/:id/image", middleware.RequireRole(middleware.RoleOwner), productHandler.UploadImage)
	}

	// ── Sales ─────────────────────────────────────────────────────────────────
	sales := protected.Group("/sales")
	{
		sales.POST("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), saleHandler.Create)
		sales.GET("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), saleHandler.List)
		sales.GET("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), saleHandler.Get)
		sales.POST("/:id/receipt", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), saleHandler.GenerateReceipt)
		sales.POST("/:id/payment", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), saleHandler.RecordPayment)
	}

	// ── Invoices ──────────────────────────────────────────────────────────────
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

	// ── Expenses ──────────────────────────────────────────────────────────────
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

	// ── Debts ─────────────────────────────────────────────────────────────────
	debts := protected.Group("/debts")
	debts.Use(middleware.SubscriptionGuard(db, "debt_tracking"))
	{
		debts.POST("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), debtHandler.Create)
		debts.GET("", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), debtHandler.List)
		debts.GET("/:id", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), debtHandler.Get)
		debts.POST("/:id/payment", middleware.RequireRole(middleware.RoleOwner, middleware.RoleStaff), debtHandler.RecordPayment)
	}

	// ── Recruitment ───────────────────────────────────────────────────────────
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

	// ── Digital products ──────────────────────────────────────────────────────
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

	// ── ADMIN: Coupon management ──────────────────────────────────────────────
	admin := protected.Group("/admin")
	admin.Use(middleware.RequirePlatformAdmin())
	{
		coupons := admin.Group("/coupons")
		{
			coupons.POST("", couponHandler.Create)
			coupons.GET("", couponHandler.List)
			coupons.GET("/:id", couponHandler.Get)
			coupons.PATCH("/:id", couponHandler.Update)
			coupons.DELETE("/:id", couponHandler.Delete)
		}
	}

	// ── SUBSCRIPTIONS ─────────────────────────────────────────────────────────
	subs := protected.Group("/subscriptions")
	{
		subs.GET("/me", subscriptionHandler.Get)
		subs.POST("/validate-coupon", subscriptionHandler.ValidateCoupon)
		subs.POST("/initiate", subscriptionHandler.Initiate)
	}
}
