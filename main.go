package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"time"
)

// APIError represents a non-2xx response from the price API,
// carrying the HTTP status, response body, and when it occurred.
type APIError struct {
	StatusCode int
	Message    string
	Timestamp  time.Time
}

// Error implements the error interface for APIError.
func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s (at %s)", e.StatusCode, e.Message, e.Timestamp.Format("15:04:05"))
}

// RateLimiter enforces a minimum spacing between consecutive requests
// so we stay under the API's requests-per-minute quota.
type RateLimiter struct {
	lastRequest time.Time
	minInterval time.Duration
}

// NewRateLimiter builds a RateLimiter that allows at most
// requestsPerMinute calls per minute.
func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	interval := time.Minute / time.Duration(requestsPerMinute)
	return &RateLimiter{minInterval: interval}
}

// Wait blocks, if necessary, until enough time has passed since the
// last request to respect minInterval, then records the new request time.
func (rl *RateLimiter) Wait() {
	elapsed := time.Since(rl.lastRequest)
	if elapsed < rl.minInterval {
		time.Sleep(rl.minInterval - elapsed)
	}
	rl.lastRequest = time.Now()
}

// fetchPriceWithRetry calls fetchPrices, retrying on failure up to
// maxRetries times. Rate-limit (429) errors get longer, linearly
// increasing backoff; other errors get a shorter backoff. Returns the
// last error encountered if all attempts fail.
func fetchPriceWithRetry(limiter *RateLimiter, maxRetries int) (*PriceResponse, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		limiter.Wait()

		prices, err := fetchPrices()
		if err == nil {
			return prices, nil
		}

		lastErr = err

		// Check if this is a rate limit error
		if apiErr, ok := err.(*APIError); ok && apiErr.StatusCode == 429 {
			// Back off longer for rate limiting, scaling with attempt count
			waitTime := time.Duration(attempt+1) * 30 * time.Second
			log.Printf("Rate limited. Waiting %v before retry %d/%d", waitTime, attempt+1, maxRetries)
			time.Sleep(waitTime)
			continue
		}
		// For other errors, wait briefly before retrying
		if attempt < maxRetries-1 {
			waitTime := time.Duration(attempt+1) * 5 * time.Second
			log.Printf("Request failed. Retrying in %v (attempt %d/%d)", waitTime, attempt+1, maxRetries)
			time.Sleep(waitTime)
		}
	}
	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// CryptoPrice holds a single coin's USD price and 24h percent change,
// as returned by the CoinGecko "simple/price" endpoint.
type CryptoPrice struct {
	USD          float64 `json:"USD"`
	USD24hChange float64 `json:"usd_24h_change"`
}

// PriceResponse maps a coin id (e.g. "bitcoin") to its price data.
type PriceResponse map[string]CryptoPrice

// clearScreen clears the terminal, using the appropriate command
// for the host OS.
func clearScreen() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// displayDashboard clears the screen and redraws the price table,
// with coins sorted alphabetically for a stable row order.
func displayDashboard(prices *PriceResponse) {
	clearScreen()

	fmt.Println("=====================================")
	fmt.Println("         CRYPTO PRICE TRACKER")
	fmt.Println("=====================================")
	fmt.Printf("Last Updated: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	fmt.Printf("%-12s %12s %12s\n", "CRYPTO", "PRICE", "24H Change")
	fmt.Println("-------------------------------------")

	// Map iteration order is random in Go, so collect and sort the
	// keys to keep row order stable across refreshes.
	keys := make([]string, 0, len(*prices))
	for crypto := range *prices {
		keys = append(keys, crypto)
	}
	sort.Strings(keys)

	for _, crypto := range keys {
		data := (*prices)[crypto]
		// "%+9.2f" prints the sign itself (+/-), so no separate
		// indicator character is needed.
		fmt.Printf("%-12s $%10.2f %+9.2f%%\n",
			formatCryptoName(crypto),
			data.USD,
			data.USD24hChange)
	}

	fmt.Println("\n=====================================")
	fmt.Println("Press Ctrl-C to exit")
}

// formatCryptoName converts a CoinGecko coin id into a display name.
// Unrecognized ids are passed through unchanged.
func formatCryptoName(name string) string {
	switch name {
	case "bitcoin":
		return "Bitcoin"
	case "ethereum":
		return "Ethereum"
	case "litecoin":
		return "Litecoin"
	default:
		return name
	}
}

// fetchPrices makes a single request to the CoinGecko API for
// bitcoin, ethereum, and litecoin prices in USD, including 24h
// change. Returns an *APIError for non-2xx responses.
func fetchPrices() (*PriceResponse, error) {
	url := "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin,ethereum,litecoin&vs_currencies=usd&include_24hr_change=true"
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("network request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
			Timestamp:  time.Now(),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var prices PriceResponse
	err = json.Unmarshal(body, &prices)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	// Validate that we received expected data
	if len(prices) == 0 {
		return nil, errors.New("no prices found")
	}
	return &prices, nil
}

// priceUpdaterWithErrorHandling runs forever, fetching prices on a
// fixed 30s tick and pushing results/errors over the given channels.
// After 5 consecutive failures it pauses updates for 5 minutes before
// resuming, to avoid hammering a struggling API.
func priceUpdaterWithErrorHandling(priceChan chan *PriceResponse, errorChan chan error) {
	limiter := NewRateLimiter(10)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	consecutiveErrors := 0
	maxConsecutiveErrors := 5

	for {
		prices, err := fetchPriceWithRetry(limiter, 3)
		if err != nil {
			consecutiveErrors++
			errorChan <- err

			if consecutiveErrors >= maxConsecutiveErrors {
				errorChan <- fmt.Errorf("too many consecutive errors (%d), pausing updates", consecutiveErrors)
				time.Sleep(5 * time.Minute)
				consecutiveErrors = 0
			}
		} else {
			consecutiveErrors = 0
			priceChan <- prices
		}
		// Wait for the next tick regardless of success/failure,
		// so this loop runs at most once per 30s outside of retries.
		<-ticker.C
	}
}

// main starts the background price updater and loops forever,
// redrawing the dashboard whenever new prices arrive and logging
// (but not halting on) errors.
func main() {
	priceChan := make(chan *PriceResponse)
	errorChan := make(chan error)

	go priceUpdaterWithErrorHandling(priceChan, errorChan)

	var currentPrices *PriceResponse

	for {
		select {
		case prices := <-priceChan:
			currentPrices = prices
			displayDashboard(currentPrices)
		case err := <-errorChan:
			fmt.Printf("Error: %v\n", err)
			fmt.Println("Retrying in 30 seconds...")
			if currentPrices != nil {
				displayDashboard(currentPrices)
			}
		}
	}
}
