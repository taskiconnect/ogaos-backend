package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

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
	handlerPublic "ogaos-backend/internal/api/handlers/public"
	handlerRecruitment "ogaos-backend/internal/api/handlers/recruitment"
	handlerSale "ogaos-backend/internal/api/handlers/sale"
	handlerStore "ogaos-backend/internal/api/handlers/store"
	handlerSubscription "ogaos-backend/internal/api/handlers/subscription"
	handlerWebhook "ogaos-backend/internal/api/handlers/webhook"

	"ogaos-backend/internal/api/routes"
	"ogaos-backend/internal/config"
	"ogaos-backend/internal/db"

	pkgImageKit "ogaos-backend/internal/external/imagekit"
	pkgPaystack "ogaos-backend/internal/external/paystack"
	pkgRedis "ogaos-backend/internal/pkg/redis"

	svcAdminAuth "ogaos-backend/internal/service/admin_auth"
	svcAuth "ogaos-backend/internal/service/auth"
	svcBusiness "ogaos-backend/internal/service/business"
	svcCoupon "ogaos-backend/internal/service/coupon"
	svcCustomer "ogaos-backend/internal/service/customer"
	svcDebt "ogaos-backend/internal/service/debt"
	svcDigital "ogaos-backend/internal/service/digital"
	svcExpense "ogaos-backend/internal/service/expense"
	svcInvoice "ogaos-backend/internal/service/invoice"
	svcLocation "ogaos-backend/internal/service/location"
	svcProduct "ogaos-backend/internal/service/product"
	svcPublic "ogaos-backend/internal/service/public"
	svcRecruitment "ogaos-backend/internal/service/recruitment"
	svcSale "ogaos-backend/internal/service/sale"
	svcStore "ogaos-backend/internal/service/store"
	svcSubscription "ogaos-backend/internal/service/subscription"
	svcUpload "ogaos-backend/internal/service/upload"

	"ogaos-backend/internal/worker"
)

func main() {
	cfg := config.Get()

	var logger *slog.Logger
	if cfg.IsProduction() {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	slog.SetDefault(logger)

	logger.Info("starting ogaos backend", "env", cfg.Env)

	db.InitDB()
	logger.Info("database connected")

	redisClient, err := pkgRedis.NewClient(cfg.UpstashRedisURL)
	if err != nil {
		logger.Error("failed to create redis client", "error", err)
		os.Exit(1)
	}

	if err := pkgRedis.Ping(context.Background(), redisClient); err != nil {
		logger.Error("failed to connect to redis", "error", err)
		os.Exit(1)
	}
	logger.Info("redis connected")

	ikClient := pkgImageKit.NewClient(
		cfg.ImageKitPublicKey,
		cfg.ImageKitPrivateKey,
		cfg.ImageKitURLEndpoint,
	)

	paystackClient := pkgPaystack.NewClient(cfg.PaystackSecretKey)

	authService := svcAuth.NewAuthService(
		db.DB,
		[]byte(cfg.JWTSecret),
		cfg.JWTExpiry,
		cfg.JWTRefreshExpiry,
		cfg.FrontendURL,
	)

	adminAuthService := svcAdminAuth.NewAdminAuthService(
		db.DB,
		[]byte(cfg.AdminJWTSecret),
		cfg.AdminJWTExpiry,
		cfg.AdminRefreshTTL,
		cfg.FrontendURL,
	)

	locService := svcLocation.NewService()
	businessSvc := svcBusiness.NewService(db.DB)
	customerSvc := svcCustomer.NewService(db.DB)
	productSvc := svcProduct.NewService(db.DB)
	publicSvc := svcPublic.NewService(db.DB)

	receiptSender := svcSale.NewEmailReceiptSender(db.DB)
	saleSvc := svcSale.NewService(db.DB, receiptSender)

	invoiceSvc := svcInvoice.NewService(db.DB, cfg.FrontendURL)
	expenseSvc := svcExpense.NewService(db.DB)
	debtSvc := svcDebt.NewService(db.DB)
	storeSvc := svcStore.NewService(db.DB)
	recruitmentSvc := svcRecruitment.NewService(db.DB, cfg.FrontendURL)
	digitalSvc := svcDigital.NewService(db.DB, ikClient, cfg.FrontendURL, cfg.PlatformFeePercent)
	couponService := svcCoupon.NewService(db.DB)
	subscriptionSvc := svcSubscription.NewService(db.DB, cfg.FrontendURL, couponService)
	uploadSvc := svcUpload.NewService(ikClient)

	scheduler := worker.NewScheduler(db.DB, paystackClient)
	workerDone := make(chan struct{})
	scheduler.Start(workerDone)

	logger.Info("background workers started")

	isProd := cfg.IsProduction()

	authHandler := handlerAuth.NewHandler(authService, isProd, logger)
	adminAuthHandler := handlerAdminAuth.NewHandler(adminAuthService, isProd, logger)

	healthHandler := handlerHealth.NewHandler(db.DB)
	locationHandler := handlerLocation.NewHandler(locService)
	businessHandler := handlerBusiness.NewHandler(businessSvc, uploadSvc)
	customerHandler := handlerCustomer.NewHandler(customerSvc)
	productHandler := handlerProduct.NewHandler(productSvc, uploadSvc)
	publicHandler := handlerPublic.NewHandler(publicSvc)
	saleHandler := handlerSale.NewHandler(saleSvc, logger)
	invoiceHandler := handlerInvoice.NewHandler(invoiceSvc)
	expenseHandler := handlerExpense.NewHandler(expenseSvc)
	debtHandler := handlerDebt.NewHandler(debtSvc)
	storeHandler := handlerStore.NewHandler(storeSvc)
	recruitmentHandler := handlerRecruitment.NewHandler(recruitmentSvc, uploadSvc)
	digitalHandler := handlerDigital.NewHandler(digitalSvc, uploadSvc)
	couponHandler := handlerCoupon.NewHandler(couponService, logger)

	subscriptionHandler := handlerSubscription.NewHandler(
		subscriptionSvc,
		couponService,
		paystackClient,
		cfg.FrontendURL,
	)

	webhookHandler := handlerWebhook.NewHandler(
		cfg.PaystackWebhookSecret,
		cfg.FlutterwaveWebhookHash,
		digitalSvc,
		subscriptionSvc,
		scheduler.Payout(),
	)

	if isProd {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())

	if !isProd {
		r.Use(gin.Logger())
	}

	if len(cfg.TrustedProxies) > 0 {
		if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
			logger.Error("failed to set trusted proxies", "error", err)
			os.Exit(1)
		}
	}

	routes.SetupAuthRoutes(
		r,
		cfg,
		db.DB,
		redisClient,
		authHandler,
		healthHandler,
		locationHandler,
		businessHandler,
		customerHandler,
		productHandler,
		saleHandler,
		invoiceHandler,
		expenseHandler,
		debtHandler,
		storeHandler,
		recruitmentHandler,
		digitalHandler,
		webhookHandler,
		couponHandler,
		subscriptionHandler,
		adminAuthHandler,
		publicHandler,
	)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		logger.Info("server starting",
			"port", cfg.Port,
			"env", cfg.Env,
		)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutdown signal received")

	close(workerDone)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("forced shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("server exited cleanly")
}
