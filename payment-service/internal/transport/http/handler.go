package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"payment-service/internal/domain"
	"payment-service/internal/usecase"
)

// PaymentHandler is the thin delivery layer for the Payment Service.
type PaymentHandler struct {
	uc *usecase.PaymentUseCase
}

func NewPaymentHandler(uc *usecase.PaymentUseCase) *PaymentHandler {
	return &PaymentHandler{uc: uc}
}

func (h *PaymentHandler) RegisterRoutes(r *gin.Engine) {
	r.POST("/payments", h.ProcessPayment)
	r.GET("/payments/:order_id", h.GetPayment)
}

type processPaymentRequest struct {
	OrderID string `json:"order_id" binding:"required"`
	Amount  int64  `json:"amount"   binding:"required,gt=0"`
}

// POST /payments
func (h *PaymentHandler) ProcessPayment(c *gin.Context) {
	var req processPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	payment, err := h.uc.ProcessPayment(req.OrderID, req.Amount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return 201 for both Authorized and Declined so the Order Service can
	// inspect the "status" field without treating a Declined as a transport error.
	c.JSON(http.StatusCreated, payment)
}

// GET /payments/:order_id
func (h *PaymentHandler) GetPayment(c *gin.Context) {
	payment, err := h.uc.GetPaymentByOrderID(c.Param("order_id"))
	if err != nil {
		if errors.Is(err, domain.ErrPaymentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "payment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, payment)
}
