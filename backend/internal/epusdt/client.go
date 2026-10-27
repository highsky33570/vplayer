package epusdt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Config holds EPUSDT / GMPay merchant settings.
type Config struct {
	BaseURL         string
	CreateOrderPath string
	QueryOrderPath  string
	PID             string
	APIToken        string
	NotifyURL       string
	RedirectURL     string
	DefaultCurrency string
	DefaultToken    string
	DefaultNetwork  string
	HTTPClient      *http.Client
}

// CreateResult is the gateway create-transaction payload we care about.
type CreateResult struct {
	TradeID        string
	Amount         float64
	Currency       string
	ActualAmount   float64
	ReceiveAddress string
	Token          string
	Network        string
	PaymentURL     string
	ExpirationTime int64
	Raw            map[string]any
}

// Client talks to the EPUSDT gateway.
type Client struct {
	cfg Config
}

func NewClient(cfg Config) *Client {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	if strings.TrimSpace(cfg.CreateOrderPath) == "" {
		cfg.CreateOrderPath = "/payments/gmpay/v1/order/create-transaction"
	}
	if strings.TrimSpace(cfg.DefaultCurrency) == "" {
		cfg.DefaultCurrency = "usd"
	}
	return &Client{cfg: cfg}
}

func (c *Client) Enabled() bool {
	return strings.TrimSpace(c.cfg.BaseURL) != "" &&
		strings.TrimSpace(c.cfg.PID) != "" &&
		strings.TrimSpace(c.cfg.APIToken) != "" &&
		strings.TrimSpace(c.cfg.NotifyURL) != ""
}

func (c *Client) VerifyNotify(params map[string]string) bool {
	return VerifySignature(c.cfg.APIToken, c.cfg.PID, params)
}

// CreateParams is the merchant-facing checkout request.
type CreateParams struct {
	OrderID     string
	AmountUSD   float64
	Currency    string
	NotifyURL   string
	RedirectURL string
	Token       string
	Network     string
}

func (c *Client) CreateTransaction(p CreateParams) (CreateResult, error) {
	if !c.Enabled() {
		return CreateResult{}, fmt.Errorf("epusdt is not configured")
	}
	currency := strings.ToLower(strings.TrimSpace(p.Currency))
	if currency == "" {
		currency = strings.ToLower(c.cfg.DefaultCurrency)
	}
	notify := strings.TrimSpace(p.NotifyURL)
	if notify == "" {
		notify = c.cfg.NotifyURL
	}
	payload := map[string]string{
		"pid":        c.cfg.PID,
		"order_id":   p.OrderID,
		"amount":     FormatAmount(p.AmountUSD),
		"currency":   currency,
		"notify_url": notify,
	}
	token := strings.ToLower(strings.TrimSpace(firstNonEmpty(p.Token, c.cfg.DefaultToken)))
	network := strings.ToLower(strings.TrimSpace(firstNonEmpty(p.Network, c.cfg.DefaultNetwork)))
	if (token == "") != (network == "") {
		return CreateResult{}, fmt.Errorf("epusdt token and network must both be set or both empty")
	}
	if token != "" {
		payload["token"] = token
		payload["network"] = network
	}
	redirect := strings.TrimSpace(firstNonEmpty(p.RedirectURL, c.cfg.RedirectURL))
	if redirect != "" {
		payload["redirect_url"] = redirect
	}
	payload["signature"] = Sign(c.cfg.APIToken, payload)

	body, err := json.Marshal(payload)
	if err != nil {
		return CreateResult{}, err
	}
	req, err := http.NewRequest(http.MethodPost, c.endpoint(c.cfg.CreateOrderPath), bytes.NewReader(body))
	if err != nil {
		return CreateResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return CreateResult{}, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return parseCreateBody(raw, res.StatusCode)
}

func (c *Client) QueryTransaction(orderNo string) (CreateResult, error) {
	path := strings.TrimSpace(c.cfg.QueryOrderPath)
	if path == "" {
		return CreateResult{}, fmt.Errorf("epusdt query path not configured")
	}
	payload := map[string]string{
		"pid":      c.cfg.PID,
		"order_id": orderNo,
	}
	payload["signature"] = Sign(c.cfg.APIToken, payload)
	body, err := json.Marshal(payload)
	if err != nil {
		return CreateResult{}, err
	}
	req, err := http.NewRequest(http.MethodPost, c.endpoint(path), bytes.NewReader(body))
	if err != nil {
		return CreateResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return CreateResult{}, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return parseCreateBody(raw, res.StatusCode)
}

func (c *Client) endpoint(path string) string {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	path = "/" + strings.TrimLeft(path, "/")
	return base + path
}

func parseCreateBody(raw []byte, statusCode int) (CreateResult, error) {
	var envelope struct {
		StatusCode int            `json:"status_code"`
		Message    string         `json:"message"`
		Data       map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return CreateResult{}, fmt.Errorf("epusdt: invalid json (http %d): %w", statusCode, err)
	}
	code := envelope.StatusCode
	if code == 0 {
		code = statusCode
	}
	if code != 200 || envelope.Data == nil {
		msg := strings.TrimSpace(envelope.Message)
		if msg == "" {
			msg = string(raw)
		}
		return CreateResult{}, fmt.Errorf("epusdt create failed (http %d / status_code %d): %s", statusCode, code, msg)
	}
	d := envelope.Data
	out := CreateResult{Raw: d}
	out.TradeID = asString(d["trade_id"])
	out.Currency = strings.ToLower(asString(d["currency"]))
	out.ReceiveAddress = asString(d["receive_address"])
	out.Token = strings.ToLower(asString(d["token"]))
	out.Network = strings.ToLower(firstNonEmpty(asString(d["network"]), "tron"))
	out.PaymentURL = firstNonEmpty(asString(d["payment_url"]), asString(d["checkout_url"]))
	out.Amount, _ = asFloat(d["amount"])
	out.ActualAmount, _ = asFloat(d["actual_amount"])
	if v, ok := asFloat(d["expiration_time"]); ok {
		out.ExpirationTime = int64(v)
	}
	if out.PaymentURL == "" {
		return CreateResult{}, fmt.Errorf("epusdt: missing payment_url")
	}
	return out, nil
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return FormatAmount(t)
	case json.Number:
		return t.String()
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := ParseFloatAmount(t)
		return f, err == nil
	default:
		return 0, false
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
