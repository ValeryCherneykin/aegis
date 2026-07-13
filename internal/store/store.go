// Package store stands in for whatever real system holds your sensitive
// data (a database, a payments provider, a CRM). It is the only place in
// the whole project real values are read from disk — swap this package
// for a real DB client and nothing else in Aegis needs to change.
package store

import (
	"encoding/json"
	"fmt"
	"os"
)

// Customer represents a user's profile and history in the real database.
type Customer struct {
	CustomerID       string  `json:"customer_id"`
	AccountAgeYears  float64 `json:"account_age_years"`
	RefundCountMonth float64 `json:"refund_count_month"`
	Balance          float64 `json:"balance"`
}

// RefundRequest represents an incoming business request to refund money.
type RefundRequest struct {
	SessionID  string  `json:"session_id"`
	CustomerID string  `json:"customer_id"`
	Amount     float64 `json:"amount"`
	Reason     string  `json:"reason"`
	Message    string  `json:"message"`
}

type db struct {
	Customers map[string]Customer      `json:"customers"`
	Requests  map[string]RefundRequest `json:"requests"`
}

// Store provides read-only access to the mock database.
type Store struct {
	data db
}

// Load reads and parses the mock database from a JSON file.
func Load(path string) (*Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mock db: %w", err)
	}
	var d db
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("parse mock db: %w", err)
	}
	return &Store{data: d}, nil
}

// Lookup returns the real request and the real customer behind it. This
// is the ONLY function in the codebase allowed to hand back raw amounts —
// callers must be inside the trust boundary (context.Categorize or
// executor.Run), never the Agent.
func (s *Store) Lookup(requestID string) (RefundRequest, Customer, error) {
	req, ok := s.data.Requests[requestID]
	if !ok {
		return RefundRequest{}, Customer{}, fmt.Errorf("unknown request_id %q", requestID)
	}
	cust, ok := s.data.Customers[req.CustomerID]
	if !ok {
		return RefundRequest{}, Customer{}, fmt.Errorf("unknown customer_id %q", req.CustomerID)
	}
	return req, cust, nil
}
