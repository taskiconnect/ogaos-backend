package payout

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	svcPayout "ogaos-backend/internal/service/payout"
)

type Handler struct {
	service *svcPayout.Service
}

func NewHandler(service *svcPayout.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListBanks(c *gin.Context) {
	result, err := h.service.ListBanks()
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, result)
}

func (h *Handler) ResolveAccount(c *gin.Context) {
	var req svcPayout.ResolveAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.service.ResolveAccount(req)
	if err != nil {
		response.Err(c, err)
		return
	}

	response.OK(c, result)
}

func (h *Handler) StartVerification(c *gin.Context) {
	var req svcPayout.StartVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.service.StartVerification(shared.MustBusinessID(c), req)
	if err != nil {
		response.Err(c, err)
		return
	}

	response.OK(c, gin.H{
		"message": "otp sent successfully",
		"data":    result,
	})
}

func (h *Handler) ResendVerification(c *gin.Context) {
	var req struct {
		VerificationID string `json:"verification_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	id, err := uuid.Parse(req.VerificationID)
	if err != nil {
		response.BadRequest(c, "invalid verification_id")
		return
	}

	result, err := h.service.ResendVerification(shared.MustBusinessID(c), id)
	if err != nil {
		response.Err(c, err)
		return
	}

	response.OK(c, gin.H{
		"message": "otp resent successfully",
		"data":    result,
	})
}

func (h *Handler) ConfirmVerification(c *gin.Context) {
	var raw struct {
		VerificationID string `json:"verification_id"`
		OTP            string `json:"otp"`
	}
	if err := c.ShouldBindJSON(&raw); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	verificationID, err := uuid.Parse(raw.VerificationID)
	if err != nil {
		response.BadRequest(c, "invalid verification_id")
		return
	}

	result, err := h.service.ConfirmVerification(shared.MustBusinessID(c), svcPayout.ConfirmVerificationRequest{
		VerificationID: verificationID,
		OTP:            raw.OTP,
	})
	if err != nil {
		response.Err(c, err)
		return
	}

	response.OK(c, gin.H{
		"message": "payout account saved successfully",
		"data":    result,
	})
}

func (h *Handler) GetDefaultAccount(c *gin.Context) {
	result, err := h.service.GetDefaultAccount(shared.MustBusinessID(c))
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, result)
}

func (h *Handler) GetPendingVerification(c *gin.Context) {
	result, err := h.service.GetPendingVerification(shared.MustBusinessID(c))
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, result)
}
