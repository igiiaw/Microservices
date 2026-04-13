package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"order-service/internal/domain"
)

// paymentRequest mirrors the Payment Service POST /payments contract.
type paymentRequest struct {
	OrderID string `json:"order_id"`
	Amount  int64  `json:"amount"` // cents, int64 – never float
}

// paymentResponse mirrors the Payment Service response body.
type paymentResponse struct {
	ID            string `json:"id"`
	OrderID       string `json:"order_id"`
	TransactionID string `json:"transaction_id"`
	Amount        int64  `json:"amount"`
	Status        string `json:"status"` // "Authorized" | "Declined"
}

// PaymentClient is the concrete adapter that implements domain.PaymentClient.
// The http.Client is injected – its Timeout (max 2 s) is set at the Composition Root.
type PaymentClient struct {
	httpClient *http.Client
	baseURL    string
}

func NewPaymentClient(httpClient *http.Client, baseURL string) *PaymentClient {
	return &PaymentClient{httpClient: httpClient, baseURL: baseURL}
}

// AuthorizePayment calls POST /payments on the Payment Service.
// Returns domain.ErrPaymentServiceUnavailable on network/timeout errors.
// Returns domain.ErrPaymentDeclined when the Payment Service explicitly declines.
func (c *PaymentClient) AuthorizePayment(orderID string, amount int64) (string, error) {
	reqBody, _ := json.Marshal(paymentRequest{OrderID: orderID, Amount: amount})

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/payments", c.baseURL),
		"application/json",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		// Covers: timeout (context deadline exceeded), connection refused, DNS failure.
		return "", domain.ErrPaymentServiceUnavailable
	}
	defer resp.Body.Close()

	// Any 5xx from the Payment Service is treated as unavailability.
	if resp.StatusCode >= 500 {
		return "", domain.ErrPaymentServiceUnavailable
	}

	var payResp paymentResponse
	if err := json.NewDecoder(resp.Body).Decode(&payResp); err != nil {
		return "", domain.ErrPaymentServiceUnavailable
	}

	if payResp.Status == "Declined" {
		return "", domain.ErrPaymentDeclined
	}

	return payResp.TransactionID, nil
}
