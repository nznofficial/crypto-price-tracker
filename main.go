package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type PriceData struct {
	Bitcoin struct {
		USD float64 `json:"usd"`
	} `json:"bitcoin"`
	Ethereum struct {
		USD float64 `json:"usd"`
	} `json:"ethereum"`
}

func main() {
	url := "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin,ethereum&vs_currencies=usd"
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error making request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading body: %v\n", err)
		return
	}
	fmt.Printf("Response: %s\n", body)

	var prices PriceData
	err = json.Unmarshal(body, &prices)
	if err != nil {
		fmt.Printf("Error parsing json: %v\n", err)
		return
	}
	fmt.Printf("Bitcoin: $%.2f\n", prices.Bitcoin.USD)
	fmt.Printf("Ethereum: $%.2f\n", prices.Ethereum.USD)
}
