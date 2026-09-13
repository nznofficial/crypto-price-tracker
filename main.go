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
	"time"
)

type APIError struct {
	StatusCode int
	Message    string
	Timestamp  time.Time
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Timestamp.Format("15:04:05"), e.Message)
}

type RateLimiter struct {
	lastRequest time.Time
	minInterval time.Duration
}

func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	interval := time.Minute / time.Duration(requestsPerMinute)
	return &RateLimiter{minInterval: interval}
}

func (rl *RateLimiter) Wait() {
	elapsed := time.Since(rl.lastRequest)
	if elapsed < rl.minInterval {
		time.Sleep(rl.minInterval - elapsed)
	}
	rl.lastRequest = time.Now()
}

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
	return nil, fmt.Errorf("failed after %d retries", maxRetries, lastErr)
}

type CryptoPrice struct {
	USD          float64 `json:"USD"`
	USD24hChange float64 `json:"usd_24h_change"`
}

type PriceResponse map[string]CryptoPrice

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

func displayDashboard(prices *PriceResponse) {
	clearScreen()

	fmt.Println("=====================================")
	fmt.Println("         CRYPTO PRICE TRACKER")
	fmt.Println("=====================================")
	fmt.Printf("Last Updated: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	fmt.Printf("%-12s %12s %12s\n", "CRYPTO", "PRICE", "24H Change")
	fmt.Println("-------------------------------------")

	for crypto, data := range *prices {
		changeIndicator := getChangeIndicator(data.USD24hChange)
		fmt.Printf("%-12s $%10.2f %s%9.2f%%\n",
			formatCryptoName(crypto),
			data.USD,
			changeIndicator,
			data.USD24hChange)
	}

	fmt.Println("\n=====================================")
	fmt.Println("Press Ctrl-C to exit")
}

func getChangeIndicator(change float64) string {
	if change > 0 {
		return "+"
	} else if change < 0 {
		return "-"
	}
	return " "
}

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
		<-ticker.C
	}
}

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
