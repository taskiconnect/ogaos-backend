// internal/api/handlers/expense/handler.go
package expense

import (
	"github.com/gin-gonic/gin"

	"ogaos-backend/internal/api/handlers/shared"
	"ogaos-backend/internal/api/response"
	svcExpense "ogaos-backend/internal/service/expense"
)

type Handler struct{ service *svcExpense.Service }

func NewHandler(s *svcExpense.Service) *Handler { return &Handler{service: s} }

// POST /expenses
func (h *Handler) Create(c *gin.Context) {
	var req svcExpense.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	exp, err := h.service.Create(shared.MustBusinessID(c), shared.MustUserID(c), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.Created(c, exp)
}

// GET /expenses
func (h *Handler) List(c *gin.Context) {
	page, limit := shared.Paginate(c)
	expenses, total, err := h.service.List(shared.MustBusinessID(c), svcExpense.ListFilter{
		StoreID:     shared.QueryUUID(c, "store_id"),
		ExpenseType: c.Query("expense_type"),
		Category:    c.Query("category"),
		DateFrom:    shared.QueryTime(c, "date_from"),
		DateTo:      shared.QueryTime(c, "date_to"),
		Page:        page,
		Limit:       limit,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.List(c, expenses, total, page, limit)
}

// GET /expenses/summary?year=2025&month=1
// NOTE: registered before /expenses/:id so the router sees /summary first
func (h *Handler) MonthlySummary(c *gin.Context) {
	year := shared.QueryInt(c, "year", 0)
	month := shared.QueryInt(c, "month", 0)
	if year == 0 || month == 0 {
		response.BadRequest(c, "year and month query params are required")
		return
	}
	summary, err := h.service.MonthlySummary(shared.MustBusinessID(c), year, month)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, summary)
}

// GET /expenses/:id
func (h *Handler) Get(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	exp, err := h.service.Get(shared.MustBusinessID(c), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, exp)
}

// PATCH /expenses/:id
func (h *Handler) Update(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	var req svcExpense.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	exp, err := h.service.Update(shared.MustBusinessID(c), id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, exp)
}

// DELETE /expenses/:id
func (h *Handler) Delete(c *gin.Context) {
	id, ok := shared.ParseID(c, "id")
	if !ok {
		return
	}
	if err := h.service.Delete(shared.MustBusinessID(c), id); err != nil {
		response.Err(c, err)
		return
	}
	response.Message(c, "expense deleted")
}
