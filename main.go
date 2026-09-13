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
	USD float64 `json:"USD"`
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
	fmt.Println("         CRYPTO PRICE TRACKER        ")
	fmt.Println("=====================================")
	fmt.Printf("Last Updated: %s\n\n", time.Now().Format("15:04:05"))

	for crypto, price := range *prices {
		fmt.Printf("%-12s $%10.2f USD\n", formatCryptoName(crypto), price.USD)
	}

	fmt.Println("\n=====================================")
	fmt.Println("Press Ctrl-C to exit")
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
	url := "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin,ethereum&vs_currencies=usd"
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

func main() {

}
