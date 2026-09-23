package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"
)

var baseURL = envOr("CLIENT_BASE_URL", "http://localhost:3001")

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func randomSleep() {
	seconds := 5 + rand.Intn(6) // 5-10 seconds
	time.Sleep(time.Duration(seconds) * time.Second)
}

func post(path string, body any) (map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(baseURL+path, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s -> %d: %s", path, resp.StatusCode, string(raw))
	}

	var result map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func newOrderPayload(orderID string) map[string]any {
	return map[string]any{
		"order_id":  orderID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"status":    "PLACED",
		"customer": map[string]any{
			"id":   "usr_99214",
			"tier": "PLATINUM",
			"demographics": map[string]any{
				"age":      34,
				"segments": []string{"tech-early-adopter", "outdoor-enthusiast", "frequent-shopper"},
			},
			"session_context": map[string]any{
				"ip_address": "192.168.1.45",
				"device":     "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X)",
				"geo_location": map[string]any{
					"country":     "US",
					"region":      "CA",
					"coordinates": []float64{37.7749, -122.4194},
				},
			},
		},
		"financials": map[string]any{
			"currency": "USD",
			"amounts": map[string]any{
				"subtotal": 1249.98,
				"tax":      103.12,
				"shipping": 0.00,
				"discounts": []map[string]any{
					{"code": "SUMMER26", "type": "percentage", "value": 0.10, "applied_to": "items"},
					{"code": "LOYALTY_CREDIT", "type": "fixed", "value": 25.00, "applied_to": "total"},
				},
				"final_total": 1103.10,
			},
		},
		"line_items": []any{},
		"polymorphic_metadata": map[string]any{
			"requires_signature": true,
			"gift_wrap": map[string]any{
				"requested":    true,
				"message":      "Happy Birthday, Alex!",
				"ribbon_color": "Gold",
			},
			"third_party_sync": map[string]any{
				"salesforce_lead_id": "00Q8000000wE4rE",
				"hubspot_deal_idx":   988124,
				"legacy_erp_payload_dump": map[string]any{
					"raw_string_blob": "ERR_04:SKU_MATCH_WARN;BATCH_77A",
					"retry_count":     3,
				},
			},
		},
	}
}

func lineItems() []map[string]any {
	return []map[string]any{
		{
			"item_id":  "prod_4412",
			"sku":      "TECH-LAP-002",
			"category": "Electronics > Computers",
			"price":    1199.99,
			"quantity": 1,
			"tags":     []string{"ultra-wide", "m3-chip", "refurbished"},
			"specifications": map[string]any{
				"ram":     "16GB",
				"storage": "512GB SSD",
				"display": map[string]any{
					"size":            "14.2 inches",
					"resolution":      "3024x1964",
					"refresh_rate_hz": 120,
				},
			},
		},
		{
			"item_id":  "prod_7781",
			"sku":      "APP-CASE-B",
			"category": "Apparel > Accessories",
			"price":    49.99,
			"quantity": 1,
			"tags":     []string{"waterproof", "leather"},
			"specifications": map[string]any{
				"color":    "Midnight Black",
				"material": "Top-grain Leather",
				"dimensions": map[string]any{
					"weight_grams": 45,
					"thickness_mm": 2.1,
				},
			},
		},
	}
}

func newOrderID(workerID, iteration int) string {
	return fmt.Sprintf("ord_w%d_i%d_%d", workerID, iteration, time.Now().UnixNano())
}

func runOrderFlow(orderID string) error {
	if _, err := post("/orders", newOrderPayload(orderID)); err != nil {
		return fmt.Errorf("create order failed: %w", err)
	}
	log.Printf("[%s] order created\n", orderID)
	randomSleep()

	for _, item := range lineItems() {
		if _, err := post(fmt.Sprintf("/orders/%s/add-items", orderID), map[string]any{
			"line_items": []map[string]any{item},
		}); err != nil {
			return fmt.Errorf("add item failed: %w", err)
		}
		log.Printf("[%s] item added: %s\n", orderID, item["sku"])
		randomSleep()
	}

	if _, err := post(fmt.Sprintf("/orders/%s/make-payment", orderID), map[string]any{
		"method": "ApplePay",
	}); err != nil {
		return fmt.Errorf("make payment failed: %w", err)
	}
	log.Printf("[%s] payment initiated\n", orderID)
	randomSleep()

	if _, err := post(fmt.Sprintf("/orders/%s/payment-webhook", orderID), map[string]any{
		"transaction_id": "txn_" + fmt.Sprint(rand.Intn(1_000_000)),
	}); err != nil {
		return fmt.Errorf("payment webhook failed: %w", err)
	}
	log.Printf("[%s] payment confirmed\n", orderID)
	randomSleep()

	if _, err := post(fmt.Sprintf("/orders/%s/start-delivery", orderID), map[string]any{
		"carrier":         "UPS",
		"tracking_number": "1Z" + fmt.Sprint(rand.Intn(1_000_000_000)),
	}); err != nil {
		return fmt.Errorf("start delivery failed: %w", err)
	}
	log.Printf("[%s] delivery started\n", orderID)
	randomSleep()

	if _, err := post(fmt.Sprintf("/orders/%s/complete-order", orderID), map[string]any{}); err != nil {
		return fmt.Errorf("complete order failed: %w", err)
	}
	log.Printf("[%s] order completed\n", orderID)

	return nil
}

const (
	numWorkers          = 100
	iterationsPerWorker = 1000
)

func worker(workerID int, wg *sync.WaitGroup) {
	defer wg.Done()

	for i := 0; i < iterationsPerWorker; i++ {
		orderID := newOrderID(workerID, i)
		if err := runOrderFlow(orderID); err != nil {
			log.Printf("[%s] flow failed: %v\n", orderID, err)
		}
	}
}

func main() {
	log.Println("waiting 5s for the backend to be ready...")
	time.Sleep(5 * time.Second)

	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for w := 0; w < numWorkers; w++ {
		go worker(w, &wg)
	}
	wg.Wait()
}
