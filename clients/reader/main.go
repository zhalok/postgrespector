package main

import (
	"encoding/json"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

var baseURL = envOr("CLIENT_BASE_URL", "http://localhost:3001")

const (
	numWorkers  = 100
	pollGap     = 2 * time.Second
	httpTimeout = 20 * time.Second
)

// statuses mirrors the order lifecycle in server/services/orders.service.js
// (PLACED -> PAYMENT_PROCESSING -> PAYMENT_SUCCESSFUL -> DELIVERING -> COMPLETED).
var statuses = []string{
	"PLACED",
	"PAYMENT_PROCESSING",
	"PAYMENT_SUCCESSFUL",
	"DELIVERING",
	"COMPLETED",
}

func randomStatus() string {
	return statuses[rand.Intn(len(statuses))]
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var httpClient = &http.Client{Timeout: httpTimeout}

type ordersPage struct {
	NextCursor *string `json:"next_cursor"`
}

// getOrdersPage fetches one page (1000 rows, index-based keyset pagination on
// timestamp, filtered by status) and returns the cursor for the next page, or
// "" once exhausted.
func getOrdersPage(cursor, status string) (string, error) {
	query := url.Values{}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	if status != "" {
		query.Set("status", status)
	}

	reqURL := baseURL + "/orders"
	if encoded := query.Encode(); encoded != "" {
		reqURL += "?" + encoded
	}

	resp, err := httpClient.Get(reqURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		io.Copy(io.Discard, resp.Body)
		return "", &httpStatusError{resp.StatusCode}
	}

	var page ordersPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return "", err
	}

	if page.NextCursor == nil {
		return "", nil
	}
	return *page.NextCursor, nil
}

// scanAllOrders pages through the entire table via keyset pagination, filtered
// by a randomly chosen status for the whole scan, so each read call touches
// every matching row instead of repeatedly hitting the same cached page.
func scanAllOrders() (int, error) {
	status := randomStatus()
	cursor := ""
	pages := 0
	for {
		next, err := getOrdersPage(cursor, status)
		if err != nil {
			return pages, err
		}
		pages++
		if next == "" {
			return pages, nil
		}
		cursor = next
	}
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string {
	return "GET /orders -> unexpected status " + http.StatusText(e.code)
}

func worker(workerID int) {
	for {
		pages, err := scanAllOrders()
		if err != nil {
			log.Printf("[reader-%d] full scan failed after %d page(s): %v\n", workerID, pages, err)
		} else {
			log.Printf("[reader-%d] full scan ok, %d page(s)\n", workerID, pages)
		}
		time.Sleep(pollGap)
	}
}

func main() {
	log.Println("waiting 5s for the backend to be ready...")
	time.Sleep(5 * time.Second)

	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for w := 0; w < numWorkers; w++ {
		go func(id int) {
			defer wg.Done()
			worker(id)
		}(w)
	}
	wg.Wait()
}
