// Package bybit implements a client against Bybit's public v5 API for
// reference pricing, and the signed v5 private API for the platform's own
// account: balance, deposit address lookup, deposit verification by tx
// hash, and withdrawal. No API keys are bundled here — every signed call
// returns ErrNotConfigured until BYBIT_API_KEY/BYBIT_API_SECRET are set.
//
// Withdrawals only work to addresses already whitelisted on Bybit's own
// site (https://www.bybit.com/user/assets/money-address) — that's a Bybit
// security control with no API bypass, so Withdraw surfaces Bybit's exact
// error rather than guessing at a "not whitelisted" condition.
package bybit

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var ErrNotConfigured = errors.New("bybit API credentials not configured")

type Client struct {
	baseURL     string
	apiKey      string
	apiSecret   string
	usdFiatRate map[string]float64 // e.g. "NGN" -> ~1500
	httpClient  *http.Client
}

func NewClient(baseURL, apiKey, apiSecret string, usdFiatRate map[string]float64) *Client {
	return &Client{
		baseURL:     baseURL,
		apiKey:      apiKey,
		apiSecret:   apiSecret,
		usdFiatRate: usdFiatRate,
		httpClient:  &http.Client{Timeout: 8 * time.Second},
	}
}

// signedRequest performs a v5 authenticated request. For GET, query is
// appended to the URL and signed as-is; for POST, body is signed and sent
// as the JSON request body. Returns the raw response bytes for the caller
// to decode into the appropriate response shape.
func (c *Client) signedRequest(ctx context.Context, method, path, query string, body []byte) ([]byte, error) {
	if c.apiKey == "" || c.apiSecret == "" {
		return nil, ErrNotConfigured
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	recvWindow := "5000"

	signaturePayload := timestamp + c.apiKey + recvWindow
	if method == http.MethodGet {
		signaturePayload += query
	} else {
		signaturePayload += string(body)
	}
	mac := hmac.New(sha256.New, []byte(c.apiSecret))
	mac.Write([]byte(signaturePayload))
	signature := hex.EncodeToString(mac.Sum(nil))

	reqURL := c.baseURL + path
	if method == http.MethodGet && query != "" {
		reqURL += "?" + query
	}

	var bodyReader io.Reader
	if method != http.MethodGet {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-BAPI-API-KEY", c.apiKey)
	req.Header.Set("X-BAPI-TIMESTAMP", timestamp)
	req.Header.Set("X-BAPI-RECV-WINDOW", recvWindow)
	req.Header.Set("X-BAPI-SIGN", signature)
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

type tickerResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		List []struct {
			Symbol    string `json:"symbol"`
			LastPrice string `json:"lastPrice"`
		} `json:"list"`
	} `json:"result"`
}

// spotPriceUSDT returns the last traded price of asset priced in USDT
// (e.g. asset="BTC" -> price of BTCUSDT).
func (c *Client) spotPriceUSDT(ctx context.Context, asset string) (float64, error) {
	symbol := asset + "USDT"
	u := fmt.Sprintf("%s/v5/market/tickers?category=spot&symbol=%s", c.baseURL, url.QueryEscape(symbol))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var out tickerResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	if out.RetCode != 0 || len(out.Result.List) == 0 {
		return 0, fmt.Errorf("bybit ticker error for %s: %s", symbol, out.RetMsg)
	}
	price, err := strconv.ParseFloat(out.Result.List[0].LastPrice, 64)
	if err != nil {
		return 0, err
	}
	return price, nil
}

// ReferencePrice implements domain.PriceFeed.
func (c *Client) ReferencePrice(ctx context.Context, asset, fiat string) (float64, error) {
	var usdtPrice float64
	if asset == "USDT" {
		usdtPrice = 1
	} else {
		p, err := c.spotPriceUSDT(ctx, asset)
		if err != nil {
			return 0, err
		}
		usdtPrice = p
	}

	switch fiat {
	case "USD", "USDT":
		return usdtPrice, nil
	default:
		rate, ok := c.usdFiatRate[fiat]
		if !ok {
			return 0, fmt.Errorf("unsupported fiat currency: %s", fiat)
		}
		return usdtPrice * rate, nil
	}
}

type walletBalanceResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		List []struct {
			Coin []struct {
				Coin          string `json:"coin"`
				WalletBalance string `json:"walletBalance"`
			} `json:"coin"`
		} `json:"list"`
	} `json:"result"`
}

// WalletBalance calls Bybit's signed v5 unified-account balance endpoint.
func (c *Client) WalletBalance(ctx context.Context) (map[string]float64, error) {
	body, err := c.signedRequest(ctx, http.MethodGet, "/v5/account/wallet-balance", "accountType=UNIFIED", nil)
	if err != nil {
		return nil, err
	}
	var out walletBalanceResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.RetCode != 0 {
		return nil, fmt.Errorf("bybit wallet-balance error: %s", out.RetMsg)
	}

	balances := map[string]float64{}
	for _, account := range out.Result.List {
		for _, coin := range account.Coin {
			bal, err := strconv.ParseFloat(coin.WalletBalance, 64)
			if err != nil {
				continue
			}
			balances[coin.Coin] = bal
		}
	}
	return balances, nil
}

type depositAddressResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		Coin   string `json:"coin"`
		Chains []struct {
			ChainType      string `json:"chainType"`
			AddressDeposit string `json:"addressDeposit"`
			TagDeposit     string `json:"tagDeposit"`
			Chain          string `json:"chain"`
		} `json:"chains"`
	} `json:"result"`
}

// DepositAddress returns Thiago's own deposit address for asset/chain. This
// is one fixed address per coin/chain for the whole account — not unique
// per user or order — so matching a specific deposit to a specific order
// relies on VerifyDeposit(txID), not on the address itself.
func (c *Client) DepositAddress(ctx context.Context, asset, chain string) (address, tag string, err error) {
	query := "coin=" + url.QueryEscape(asset)
	if chain != "" {
		query += "&chainType=" + url.QueryEscape(chain)
	}
	body, err := c.signedRequest(ctx, http.MethodGet, "/v5/asset/deposit/query-address", query, nil)
	if err != nil {
		return "", "", err
	}
	var out depositAddressResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", "", err
	}
	if out.RetCode != 0 {
		return "", "", fmt.Errorf("bybit deposit-address error: %s", out.RetMsg)
	}
	if len(out.Result.Chains) == 0 {
		return "", "", fmt.Errorf("bybit returned no deposit address for %s/%s", asset, chain)
	}
	ch := out.Result.Chains[0]
	return ch.AddressDeposit, ch.TagDeposit, nil
}

type depositRecordResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		Rows []struct {
			Coin          string `json:"coin"`
			Amount        string `json:"amount"`
			TxID          string `json:"txID"`
			Status        int    `json:"status"`
			Confirmations string `json:"confirmations"`
		} `json:"rows"`
	} `json:"result"`
}

// bybitDepositSuccessStatus is Bybit's status code for a fully credited
// deposit (see the deposit-record endpoint's status enum).
const bybitDepositSuccessStatus = 3

// VerifyDeposit looks up a deposit by transaction hash and reports the
// credited amount if Bybit shows it as successful. confirmed=false with a
// nil error means the deposit isn't visible/confirmed yet (still in
// flight) — callers should treat that as "not yet", not as failure.
func (c *Client) VerifyDeposit(ctx context.Context, asset, txID string) (amount float64, confirmed bool, err error) {
	query := "coin=" + url.QueryEscape(asset) + "&txID=" + url.QueryEscape(txID)
	body, err := c.signedRequest(ctx, http.MethodGet, "/v5/asset/deposit/query-record", query, nil)
	if err != nil {
		return 0, false, err
	}
	var out depositRecordResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, false, err
	}
	if out.RetCode != 0 {
		return 0, false, fmt.Errorf("bybit deposit-record error: %s", out.RetMsg)
	}
	for _, row := range out.Result.Rows {
		if row.TxID != txID {
			continue
		}
		if row.Status != bybitDepositSuccessStatus {
			return 0, false, nil
		}
		amt, err := strconv.ParseFloat(row.Amount, 64)
		if err != nil {
			return 0, false, err
		}
		return amt, true, nil
	}
	return 0, false, nil
}

type withdrawResponse struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		ID string `json:"id"`
	} `json:"result"`
}

// Withdraw sends asset from Thiago's Bybit account to address on chain.
// Requires the master account's API key and a pre-whitelisted address
// (configured on Bybit's site, not via API) — Bybit's exact error message
// is returned as-is when that's the problem, since there's no documented
// API-level way to distinguish "not whitelisted" from other failures.
func (c *Client) Withdraw(ctx context.Context, asset, chain, address string, amount float64) (withdrawID string, err error) {
	payload := map[string]any{
		"coin":        asset,
		"chain":       chain,
		"address":     address,
		"amount":      strconv.FormatFloat(amount, 'f', -1, 64),
		"timestamp":   time.Now().UnixMilli(),
		"forceChain":  0,
		"accountType": "FUND",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	respBody, err := c.signedRequest(ctx, http.MethodPost, "/v5/asset/withdraw/create", "", body)
	if err != nil {
		return "", err
	}
	var out withdrawResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", err
	}
	if out.RetCode != 0 {
		return "", fmt.Errorf("bybit withdraw error: %s", out.RetMsg)
	}
	return out.Result.ID, nil
}
