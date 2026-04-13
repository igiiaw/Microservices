package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"order-service/internal/domain"
	"order-service/internal/usecase"
)

// OrderHandler is the thin delivery layer.
// Its only responsibility: parse the request, call the use case, map the response.
type OrderHandler struct {
	uc *usecase.OrderUseCase
}

func NewOrderHandler(uc *usecase.OrderUseCase) *OrderHandler {
	return &OrderHandler{uc: uc}
}

func (h *OrderHandler) RegisterRoutes(r *gin.Engine) {
	r.POST("/orders", h.CreateOrder)
	r.GET("/orders/:id", h.GetOrder)
	r.PATCH("/orders/:id/cancel", h.CancelOrder)
	r.GET("/customers/:customer_id/orders", h.GetOrdersByCustomerID)
}

// createOrderRequest is the inbound DTO – kept separate from the domain model.
type createOrderRequest struct {
	CustomerID string `json:"customer_id" binding:"required"`
	ItemName   string `json:"item_name"   binding:"required"`
	Amount     int64  `json:"amount"      binding:"required,gt=0"`
}

// POST /orders
func (h *OrderHandler) CreateOrder(c *gin.Context) {
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Bonus: read optional Idempotency-Key header
	idempotencyKey := c.GetHeader("Idempotency-Key")

	order, err := h.uc.CreateOrder(idempotencyKey, req.CustomerID, req.ItemName, req.Amount)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrPaymentServiceUnavailable):
			// Required failure scenario: return 503 when Payment Service is down/timed-out.
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "payment service unavailable"})
		case errors.Is(err, domain.ErrInvalidAmount):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, domain.ErrPaymentDeclined):
			// Payment was declined: order exists but is Failed – return 201 with the order.
			// The client can inspect order.status to see "Failed".
			c.JSON(http.StatusCreated, order)
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, order)
}

// GET /orders/:id
func (h *OrderHandler) GetOrder(c *gin.Context) {
	order, err := h.uc.GetOrder(c.Param("id"))
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, order)
}

// PATCH /orders/:id/cancel
func (h *OrderHandler) CancelOrder(c *gin.Context) {
	order, err := h.uc.CancelOrder(c.Param("id"))
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrOrderNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
		case usecase.IsCancelConflict(err):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, order)
}

func (h *OrderHandler) GetOrdersByCustomerID(c *gin.Context) {
	customerID := c.Param("customer_id")
	orders, err := h.uc.GetOrdersByCustomerID(customerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, orders)
}
