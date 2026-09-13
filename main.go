package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

//type PriceData struct {
//	Bitcoin struct {
//		USD float64 `json:"usd"`
//	} `json:"bitcoin"`
//	Ethereum struct {
//		USD float64 `json:"usd"`
//	} `json:"ethereum"`
//}

type CryptoPrice struct {
	USD          float64 `json:"USD"`
	USD24hChange float64 `json:"usd_24h_change"`
}

//type ExtendedCryptoData struct {
//	USD 			float64 `json:"USD"`
//	USD24hChange 	float64 `json:"usd_24h_change"`
//	USD24hVolume 	float64 `json:"usd_24h_vol"`
//	LastUpdatedAt 	int64 	`json:"last_updated_at"`
//}

type PriceResponse map[string]CryptoPrice

//type ExtendedPriceResponse map[string]ExtendedCryptoData

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
	url := "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin,ethereum,litecoin&vs_currencies=usd&include_24r_change=true"
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var prices PriceResponse
	err = json.Unmarshal(body, &prices)
	if err != nil {
		return nil, err
	}
	return &prices, nil
}

func priceUpdater(priceChan chan *PriceResponse) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		prices, err := fetchPrices()
		if err != nil {
			fmt.Printf("Error fetching prices: %v\n", err)
			continue
		}
		priceChan <- prices
		<-ticker.C
	}
}

func main() {
	priceChan := make(chan *PriceResponse)

	go priceUpdater(priceChan)

	for {
		prices := <-priceChan
		displayDashboard(prices)
	}

}
