package subscription

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	pkgPaystack "ogaos-backend/internal/external/paystack"
	"ogaos-backend/internal/service/coupon"
	"ogaos-backend/internal/service/subscription"
)

type Handler struct {
	subscriptionService *subscription.Service
	couponService       *coupon.Service
	paystackClient      *pkgPaystack.Client
	frontendURL         string
}

func NewHandler(sub *subscription.Service, couponSvc *coupon.Service, paystack *pkgPaystack.Client, frontendURL string) *Handler {
	return &Handler{
		subscriptionService: sub,
		couponService:       couponSvc,
		paystackClient:      paystack,
		frontendURL:         frontendURL,
	}
}

func (h *Handler) Get(c *gin.Context) {
	businessID := shared.MustBusinessID(c)
	sub, err := h.subscriptionService.Get(businessID)
	if err != nil {
		response.NotFound(c, "subscription not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": sub})
}

func (h *Handler) ValidateCoupon(c *gin.Context) {
	var req struct {
		Plan           string `json:"plan" binding:"required"`
		PeriodMonths   int    `json:"period_months" binding:"required,min=1"`
		CouponCode     string `json:"coupon_code" binding:"required"`
		OriginalAmount int64  `json:"original_amount" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	data, err := h.couponService.ValidateForPlan(req.CouponCode, req.Plan, req.OriginalAmount)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func (h *Handler) Initiate(c *gin.Context) {
	businessID := shared.MustBusinessID(c)
	email := c.MustGet("email").(string)

	isPlatform, _ := c.Get("is_platform")

	var req struct {
		Plan         string  `json:"plan" binding:"required"`
		PeriodMonths int     `json:"period_months" binding:"required,min=1,max=12"`
		CouponCode   *string `json:"coupon_code"`
		CustomAmount *int64  `json:"custom_amount"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if req.Plan == "custom" && isPlatform != true {
		response.BadRequest(c, "custom plans can only be created by the sales team. Please contact sales.")
		return
	}

	data, err := h.subscriptionService.InitiatePayment(
		businessID,
		req.Plan,
		req.PeriodMonths,
		req.CouponCode,
		req.CustomAmount,
	)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if activated, _ := data["activated"].(bool); activated {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
		return
	}

	initReq := pkgPaystack.InitializeTransactionRequest{
		Email:     email,
		Amount:    data["amount"].(int64),
		Reference: data["reference"].(string),
		Callback:  h.frontendURL + "/dashboard/subscription/callback?reference=" + data["reference"].(string),
		Metadata: map[string]interface{}{
			"business_id":   businessID.String(),
			"plan":          req.Plan,
			"period_months": req.PeriodMonths,
			"type":          "subscription_upgrade",
		},
	}

	paystackResp, err := h.paystackClient.InitializeTransaction(initReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to initialize payment with Paystack"})
		return
	}

	data["paystack_authorization_url"] = paystackResp.Data.AuthorizationURL

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Redirect customer to Paystack to complete payment",
		"data":    data,
	})
}

// Verify is called by the frontend after Paystack redirects back.
// It confirms the charge with Paystack directly and activates the subscription.
// This is a safe fallback for when the webhook hasn't fired yet.
// ActivateFromSuccessfulPayment is idempotent so calling it twice is harmless.
func (h *Handler) Verify(c *gin.Context) {
	businessID := shared.MustBusinessID(c)

	var req struct {
		Reference string `json:"reference" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 1. Ensure the pending record exists and belongs to this business.
	//    This prevents one user activating another's subscription.
	pending, err := h.subscriptionService.FindPendingByReference(req.Reference)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "payment reference not found"})
		return
	}
	if pending.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "reference does not belong to your account"})
		return
	}

	// 2. Ask Paystack whether the charge actually succeeded.
	txn, err := h.paystackClient.VerifyTransaction(req.Reference)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "could not reach Paystack to verify payment"})
		return
	}
	if txn.Data.Status != "success" {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"success": false,
			"message": "payment has not been completed yet",
			"status":  txn.Data.Status,
		})
		return
	}

	// 3. Activate the subscription. Safe to call even if webhook already did it.
	sub, err := h.subscriptionService.ActivateFromSuccessfulPayment(req.Reference)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": sub})
}
