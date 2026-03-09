// cmd/api/main.go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	handlerAuth "ogaos-backend/internal/api/handlers/auth"
	handlerBusiness "ogaos-backend/internal/api/handlers/business"
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
	handlerWebhook "ogaos-backend/internal/api/handlers/webhook"
	"ogaos-backend/internal/api/routes"
	"ogaos-backend/internal/config"
	"ogaos-backend/internal/db"
	pkgImageKit "ogaos-backend/internal/external/imagekit"
	pkgPaystack "ogaos-backend/internal/external/paystack"
	svcAuth "ogaos-backend/internal/service/auth"
	svcBusiness "ogaos-backend/internal/service/business"
	svcCustomer "ogaos-backend/internal/service/customer"
	svcDebt "ogaos-backend/internal/service/debt"
	svcDigital "ogaos-backend/internal/service/digital"
	svcExpense "ogaos-backend/internal/service/expense"
	svcInvoice "ogaos-backend/internal/service/invoice"
	svcLocation "ogaos-backend/internal/service/location"
	svcProduct "ogaos-backend/internal/service/product"
	svcRecruitment "ogaos-backend/internal/service/recruitment"
	svcSale "ogaos-backend/internal/service/sale"
	svcStore "ogaos-backend/internal/service/store"
	svcSubscription "ogaos-backend/internal/service/subscription"
	svcUpload "ogaos-backend/internal/service/upload"
	"ogaos-backend/internal/worker"
)

func main() {
	// Load configuration
	cfg := config.Get()

	// Initialize database connection
	db.InitDB()
	log.Println("Database connected successfully")

	// ── External clients ──────────────────────────────────────────────────────
	ikClient := pkgImageKit.NewClient(
		cfg.ImageKitPublicKey,
		cfg.ImageKitPrivateKey,
		cfg.ImageKitURLEndpoint,
	)
	paystackClient := pkgPaystack.NewClient(cfg.PaystackSecretKey)

	// ── Services ──────────────────────────────────────────────────────────────
	authService := svcAuth.NewAuthService(
		db.DB,
		[]byte(cfg.JWTSecret),
		cfg.JWTExpiry,
		cfg.JWTRefreshExpiry,
		cfg.FrontendURL,
	)
	locService := svcLocation.NewService()
	businessSvc := svcBusiness.NewService(db.DB)
	customerSvc := svcCustomer.NewService(db.DB)
	productSvc := svcProduct.NewService(db.DB)
	saleSvc := svcSale.NewService(db.DB)
	invoiceSvc := svcInvoice.NewService(db.DB, cfg.FrontendURL)
	expenseSvc := svcExpense.NewService(db.DB)
	debtSvc := svcDebt.NewService(db.DB)
	storeSvc := svcStore.NewService(db.DB)
	recruitmentSvc := svcRecruitment.NewService(db.DB, cfg.FrontendURL)
	digitalSvc := svcDigital.NewService(db.DB, ikClient, cfg.FrontendURL, cfg.PlatformFeePercent)
	subscriptionSvc := svcSubscription.NewService(db.DB, cfg.FrontendURL)
	uploadSvc := svcUpload.NewService(ikClient)

	// ── Background workers ────────────────────────────────────────────────────
	scheduler := worker.NewScheduler(db.DB, paystackClient)
	workerDone := make(chan struct{})
	scheduler.Start(workerDone)

	// ── HTTP handlers ─────────────────────────────────────────────────────────
	authHandler := handlerAuth.NewHandler(authService)
	healthHandler := handlerHealth.NewHandler(db.DB)
	locationHandler := handlerLocation.NewHandler(locService)
	businessHandler := handlerBusiness.NewHandler(businessSvc, uploadSvc)
	customerHandler := handlerCustomer.NewHandler(customerSvc)
	productHandler := handlerProduct.NewHandler(productSvc, uploadSvc)
	saleHandler := handlerSale.NewHandler(saleSvc)
	invoiceHandler := handlerInvoice.NewHandler(invoiceSvc)
	expenseHandler := handlerExpense.NewHandler(expenseSvc)
	debtHandler := handlerDebt.NewHandler(debtSvc)
	storeHandler := handlerStore.NewHandler(storeSvc)
	recruitmentHandler := handlerRecruitment.NewHandler(recruitmentSvc, uploadSvc)
	digitalHandler := handlerDigital.NewHandler(digitalSvc, uploadSvc)
	webhookHandler := handlerWebhook.NewHandler(
		cfg.PaystackWebhookSecret,
		cfg.FlutterwaveWebhookHash,
		digitalSvc,
		subscriptionSvc,
		scheduler.Payout(),
	)

	// Set Gin mode based on environment
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	// Initialize Gin router
	r := gin.Default()

	// Setup all API routes
	routes.SetupAuthRoutes(
		r, cfg, db.DB,
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
	)

	// Create HTTP server with graceful shutdown
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Starting OgaOs API on :%s (%s mode)", cfg.Port, cfg.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal (Ctrl+C / SIGTERM)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Stop background workers first
	close(workerDone)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced shutdown: %v", err)
	}

	log.Println("Server exited gracefully")
}
