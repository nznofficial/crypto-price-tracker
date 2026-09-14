# Crypto Price Tracker

A terminal-based CLI dashboard that polls the [CoinGecko](https://www.coingecko.com/) API for live Bitcoin, Ethereum, and Litecoin prices, and redraws an aligned, auto-refreshing table every 30 seconds.

## Features

- **Live price polling** — fetches USD prices and 24h % change for BTC, ETH, and LTC on a fixed interval
- **Rate limiting** — client-side throttling to stay within API request quotas
- **Retry with backoff** — automatically retries failed requests, with longer backoff for HTTP 429 (rate limit) responses and shorter backoff for other errors
- **Graceful degradation** — after repeated consecutive failures, updates pause for a cooldown period instead of hammering the API; the last successfully fetched prices stay on screen during errors
- **Cross-platform terminal clearing** — works on both Windows and Unix-like systems
- **Stable, sorted display** — coin rows are alphabetically sorted so the table doesn't reorder itself between refreshes

## How It Works

1. A background goroutine (`priceUpdaterWithErrorHandling`) polls `fetchPrices()` every 30 seconds via a `time.Ticker`.
2. Each fetch attempt goes through `fetchPriceWithRetry`, which retries up to 3 times, applying rate-limiter-aware backoff.
3. Results (or errors) are sent over channels to the main loop.
4. `main` listens on both channels with a `select` statement, redrawing the dashboard on new data and logging errors without crashing.

## Topics / Concepts Demonstrated

| Topic | Where | Details |
|---|---|---|
| Custom error types | `APIError` implementing the `error` interface | Bundles status code, message, and timestamp into one type instead of a bare string, and satisfies `error` via `Error() string` so it can be returned anywhere `error` is expected. |
| Type assertions on errors | `fetchPriceWithRetry` | `err.(*APIError)` checks whether a generic `error` is actually an `*APIError`, letting the code branch on status code (e.g. treat 429 differently from other failures). |
| Error wrapping (`%w`) | `fetchPriceWithRetry`, `fetchPrices` | `fmt.Errorf("...: %w", err)` preserves the original error inside a new one, so callers can still use `errors.Is`/`errors.As`/`errors.Unwrap` instead of losing the root cause. |
| Rate limiting | `RateLimiter` | Tracks the time of the last request and sleeps just long enough to enforce a minimum interval, implementing a simple requests-per-minute cap without external libraries. |
| Retry logic with backoff | `fetchPriceWithRetry` | Retries failed requests up to a max count; uses a longer, attempt-scaled delay for rate-limit errors (HTTP 429) and a shorter delay for other transient errors — a basic form of exponential-ish backoff. |
| HTTP client requests | `fetchPrices` (`net/http`) | `http.Get`, checking `resp.StatusCode`, reading the body with `io.ReadAll`, and using `defer resp.Body.Close()` to guarantee cleanup even on early returns. |
| JSON decoding with struct tags | `CryptoPrice`, `PriceResponse` | `encoding/json` unmarshals API responses into typed Go structs; struct tags (`json:"USD"`, `json:"usd_24h_change"`) map mismatched JSON field names to idiomatic Go field names. |
| Map types as data structures | `PriceResponse` (`map[string]CryptoPrice`) | Uses a named map type to model a dynamic, keyed JSON object (coin id → price data) rather than a fixed struct. |
| Goroutines & channels | `priceUpdaterWithErrorHandling`, `main` | A background goroutine produces data; `main` consumes it via unbuffered channels — a classic Go producer/consumer pattern for decoupling I/O from rendering. |
| `select` statements | `main` | Listens on two channels (`priceChan`, `errorChan`) simultaneously, handling whichever one has data ready — the core Go primitive for multiplexing concurrent event sources. |
| Periodic polling with tickers | `time.NewTicker` | Drives a fixed-interval loop (30s) for polling, paired with `defer ticker.Stop()` to avoid leaking the ticker's underlying resources. |
| Cross-platform shell commands | `clearScreen` | Uses `runtime.GOOS` to branch between `cmd /c cls` (Windows) and `clear` (Unix-like), then runs it via `os/exec`. |
| Terminal table formatting | `displayDashboard` | `fmt.Printf` verb width/precision specifiers (`%-12s`, `%10.2f`, `%+9.2f`) to left/right-align columns and force explicit +/- signs on percentage changes. |
| Deterministic map iteration | sorting keys before ranging | Go randomizes map iteration order by design; collecting keys into a slice and calling `sort.Strings` keeps the displayed row order stable across refreshes. |
| Graceful degradation | consecutive-error cooldown, stale-data fallback | After repeated consecutive failures the updater pauses (rather than retrying indefinitely), and `main` keeps showing the last successfully fetched prices instead of a blank screen during errors. |
| Pointer semantics | `*PriceResponse`, `*APIError`, `*RateLimiter` | Functions accept/return pointers to avoid copying larger structs and to allow shared mutable state (e.g. `RateLimiter.lastRequest`) to persist across calls. |
| Package organization / stdlib usage | imports block | Relies entirely on the standard library (`net/http`, `encoding/json`, `time`, `sort`, `os/exec`, `runtime`) with no third-party dependencies — a good example of what's achievable without external packages. |

## Requirements

- Go 1.18+ (or any version supporting the standard library used here)
- Internet access to reach `api.coingecko.com`

## Running

```bash
go run main.go
```

Press `Ctrl-C` to exit.

## Example Output

```
=====================================
         CRYPTO PRICE TRACKER
=====================================
Last Updated: 2026-09-13 14:32:07

CRYPTO             PRICE   24H Change
-------------------------------------
Bitcoin       $ 64230.12    +2.14%
Ethereum      $  3120.45    -0.87%
Litecoin      $    88.30    +0.42%

=====================================
Press Ctrl-C to exit
```

## Notes

- The API endpoint and polling interval are currently hardcoded; consider extracting them into configuration/flags for reuse with other coins or intervals.
- No persistent storage — prices exist only in memory for the current run.