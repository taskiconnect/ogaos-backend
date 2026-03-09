// internal/api/handlers/location/handler.go
package location

import (
	"net/http"
	"strings"

	"ogaos-backend/internal/service/location"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *location.Service
}

func NewHandler(service *location.Service) *Handler {
	return &Handler{service: service}
}

// GetStates returns all Nigerian states
// GET /locations/states
func (h *Handler) GetStates(c *gin.Context) {
	states := h.service.GetStates()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    states,
	})
}

// GetLGAs returns all LGAs for a given state
// GET /locations/lgas?state=Oyo
func (h *Handler) GetLGAs(c *gin.Context) {
	state := strings.TrimSpace(c.Query("state"))
	if state == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "state query parameter is required",
		})
		return
	}

	lgas, found := h.service.GetLGAs(state)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "state not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"state":   state,
		"data":    lgas,
	})
}
