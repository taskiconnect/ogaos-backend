// internal/api/handlers/auth/list_staff.go
// CREATE this as a NEW file in the internal/api/handlers/auth/ folder
package auth

import (
	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
)

// GET /staff
func (h *Handler) ListStaff(c *gin.Context) {
	staff, err := h.service.ListStaff(shared.MustBusinessID(c))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, staff)
}
