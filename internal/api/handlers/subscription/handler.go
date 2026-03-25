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

	var req struct {
		Plan         string  `json:"plan" binding:"required"`
		PeriodMonths int     `json:"period_months" binding:"required,min=1,max=12"`
		CouponCode   *string `json:"coupon_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	data, err := h.subscriptionService.InitiatePayment(businessID, req.Plan, req.PeriodMonths, req.CouponCode)
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
