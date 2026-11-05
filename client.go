package rpay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Error represents a structured error returned by the RPAY SDK or remote API.
type Error struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	StatusCode int            `json:"statusCode,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("rpay [%s] (HTTP %d): %s", e.Code, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("rpay [%s]: %s", e.Code, e.Message)
}

// Config holds configuration for the RPAY Client.
// Any empty fields automatically fall back to RPAY_* environment variables.
type Config struct {
	BaseURL       string
	Domain        string
	SiteID        string
	SiteDomain    string
	BankToken     string
	BankCode      string
	AccountNumber string
	Password      string
	USDTMode      string // "public" or "stc"
	USDTAPIKey    string
	USDTSecretKey string
	USDTNetwork   string
	WebhookSecret string
	Timeout       time.Duration
	Headers       map[string]string
	HTTPClient    *http.Client
}

// Option configures a Config instance.
type Option func(*Config)

// WithDomain sets the target RPAY domain or base URL (supports root or white-label child domains).
func WithDomain(domainOrURL string) Option {
	return func(c *Config) {
		c.Domain = domainOrURL
		c.BaseURL = domainOrURL
	}
}

// WithSiteID sets the explicit X-Site-ID header for multi-tenant child sites.
func WithSiteID(siteID string) Option {
	return func(c *Config) {
		c.SiteID = siteID
	}
}

// WithBankToken sets the default Bank API token and optional bank code, account number, and password.
func WithBankToken(token string, bankCode, accountNumber, password string) Option {
	return func(c *Config) {
		c.BankToken = token
		if bankCode != "" {
			c.BankCode = bankCode
		}
		if accountNumber != "" {
			c.AccountNumber = accountNumber
		}
		if password != "" {
			c.Password = password
		}
	}
}

// WithUSDT configures USDT/USDC credentials and default network ("TRC20", "BEP20", etc.).
func WithUSDT(apiKey, secretKey, network, mode string) Option {
	return func(c *Config) {
		c.USDTAPIKey = apiKey
		c.USDTSecretKey = secretKey
		if network != "" {
			c.USDTNetwork = network
		}
		if mode != "" {
			c.USDTMode = mode
		}
	}
}

// WithWebhookSecret sets the default HMAC-SHA256 webhook secret.
func WithWebhookSecret(secret string) Option {
	return func(c *Config) {
		c.WebhookSecret = secret
	}
}

// WithHTTPClient provides a custom *http.Client (useful for testing or custom transport).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Config) {
		c.HTTPClient = hc
	}
}

func normalizeDomainAndURL(domainOrURL string) (baseURL string, host string) {
	raw := strings.TrimSpace(domainOrURL)
	if raw == "" {
		raw = DefaultBaseURL
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	raw = strings.TrimRight(raw, "/")
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return DefaultBaseURL, ""
	}
	return raw, strings.ToLower(parsed.Host)
}

func envOr(val string, envKeys ...string) string {
	if strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	for _, k := range envKeys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// Client is a thread-safe RPAY SDK client supporting Multi-Domain and Unified Bank + USDT operations.
type Client struct {
	cfg        Config
	baseURL    string
	siteDomain string
	httpClient *http.Client
}

// New creates a new RPAY Client, automatically loading defaults from RPAY_* environment variables.
func New(opts ...Option) *Client {
	cfg := Config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return NewWithConfig(cfg)
}

// NewWithConfig creates a new RPAY Client from an explicit Config struct with environment variable fallback.
func NewWithConfig(cfg Config) *Client {
	rawDomain := firstNonEmpty(cfg.BaseURL, cfg.Domain, os.Getenv("RPAY_BASE_URL"), os.Getenv("RPAY_DOMAIN"), DefaultBaseURL)
	baseURL, detectedHost := normalizeDomainAndURL(rawDomain)

	cfg.BaseURL = baseURL
	cfg.SiteID = envOr(cfg.SiteID, "RPAY_SITE_ID")
	cfg.SiteDomain = envOr(cfg.SiteDomain, "RPAY_SITE_DOMAIN")
	if cfg.SiteDomain == "" {
		cfg.SiteDomain = detectedHost
	}
	cfg.BankToken = envOr(cfg.BankToken, "RPAY_BANK_TOKEN", "RPAY_TOKEN")
	cfg.BankCode = strings.ToUpper(firstNonEmpty(envOr(cfg.BankCode, "RPAY_BANK_CODE"), "ACB"))
	cfg.AccountNumber = envOr(cfg.AccountNumber, "RPAY_ACCOUNT_NUMBER")
	cfg.Password = envOr(cfg.Password, "RPAY_BANK_PASSWORD")
	cfg.USDTAPIKey = envOr(cfg.USDTAPIKey, "RPAY_USDT_API_KEY", "RPAY_API_KEY")
	cfg.USDTSecretKey = envOr(cfg.USDTSecretKey, "RPAY_USDT_SECRET_KEY", "RPAY_SECRET_KEY")
	cfg.USDTNetwork = strings.ToUpper(firstNonEmpty(envOr(cfg.USDTNetwork, "RPAY_USDT_NETWORK"), "TRC20"))
	cfg.USDTMode = strings.ToLower(firstNonEmpty(envOr(cfg.USDTMode, "RPAY_USDT_MODE"), "public"))
	cfg.WebhookSecret = envOr(cfg.WebhookSecret, "RPAY_WEBHOOK_SECRET")
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}

	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout}
	}

	return &Client{
		cfg:        cfg,
		baseURL:    baseURL,
		siteDomain: cfg.SiteDomain,
		httpClient: hc,
	}
}

// ForDomain clones the client bound to a specific domain or child site while preserving credentials.
func (c *Client) ForDomain(domainOrURL string, opts ...Option) *Client {
	nextCfg := c.cfg
	nextCfg.BaseURL = domainOrURL
	nextCfg.Domain = domainOrURL
	nextCfg.SiteDomain = ""
	for _, opt := range opts {
		if opt != nil {
			opt(&nextCfg)
		}
	}
	return NewWithConfig(nextCfg)
}

func (c *Client) doRequest(ctx context.Context, method, path string, query map[string]string, jsonBody any, formBody map[string]string, customHeaders map[string]string) (map[string]any, error) {
	u := c.baseURL + "/" + strings.TrimLeft(path, "/")
	if len(query) > 0 {
		q := url.Values{}
		for k, v := range query {
			if v != "" {
				q.Set(k, v)
			}
		}
		if encoded := q.Encode(); encoded != "" {
			u += "?" + encoded
		}
	}

	var bodyReader io.Reader
	contentType := ""
	if jsonBody != nil {
		b, err := json.Marshal(jsonBody)
		if err != nil {
			return nil, &Error{Code: "SERIALIZATION_ERROR", Message: err.Error()}
		}
		bodyReader = bytes.NewReader(b)
		contentType = "application/json"
	} else if len(formBody) > 0 {
		vals := url.Values{}
		for k, v := range formBody {
			if v != "" {
				vals.Set(k, v)
			}
		}
		bodyReader = strings.NewReader(vals.Encode())
		contentType = "application/x-www-form-urlencoded"
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return nil, &Error{Code: "REQUEST_BUILD_ERROR", Message: err.Error()}
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "rpay-sdk-go/1.0.0")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.siteDomain != "" {
		req.Header.Set("X-Site-Domain", c.siteDomain)
	}
	if c.cfg.SiteID != "" {
		req.Header.Set("X-Site-ID", c.cfg.SiteID)
	}
	for k, v := range c.cfg.Headers {
		req.Header.Set(k, v)
	}
	for k, v := range customHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &Error{Code: "NETWORK_ERROR", Message: err.Error()}
	}
	defer resp.Body.Close()

	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &Error{Code: "READ_BODY_ERROR", Message: err.Error(), StatusCode: resp.StatusCode}
	}

	var parsed map[string]any
	if len(rawBytes) > 0 {
		_ = json.Unmarshal(rawBytes, &parsed)
	}
	if parsed == nil {
		parsed = map[string]any{"raw": string(rawBytes)}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := firstNonEmpty(toString(parsed["message"]), toString(parsed["msg"]), toString(parsed["error"]), fmt.Sprintf("HTTP %d", resp.StatusCode))
		return nil, &Error{
			Code:       firstNonEmpty(toString(parsed["code"]), "HTTP_ERROR"),
			Message:    msg,
			StatusCode: resp.StatusCode,
			Details:    parsed,
		}
	}

	return parsed, nil
}

// PayRequest represents parameters for creating a Bank (VND) or Crypto (USDT/USDC) invoice.
type PayRequest struct {
	Channel        string            // "bank" or "usdt" (auto-detected if empty)
	Amount         float64           // Amount in VND or USDT/USDC
	OrderID        string            // Merchant order ID
	Currency       string            // "VND", "USDT", "USDC"
	Network        string            // "TRC20", "BEP20", "POLYGON", "ARB", "TON", etc.
	Mode           string            // "public" or "stc" for USDT
	Token          string            // Override Bank Token
	APIKey         string            // Override USDT API Key
	SecretKey      string            // Override USDT Secret Key
	Description    string            // Payment description
	BuyerName      string            // Optional buyer name
	BuyerEmail     string            // Optional buyer email
	BuyerPhone     string            // Optional buyer phone
	CallbackURL    string            // Webhook callback URL
	ReturnURL      string            // Return URL after payment
	CancelURL      string            // Cancel URL
	ExpiresInHours int               // Invoice expiration in hours
	IdempotencyKey string            // Optional idempotency key
	Headers        map[string]string // Optional extra headers
}

// Pay creates a unified payment invoice for either Bank (VietQR VND) or Crypto (USDT/USDC).
func (c *Client) Pay(ctx context.Context, req PayRequest) (UnifiedInvoiceDTO, error) {
	ch := DetectChannel(req.Channel, req.Currency, req.Network)
	if ch == "usdt" {
		mode := strings.ToLower(firstNonEmpty(req.Mode, c.cfg.USDTMode, "public"))
		netInfo, ok := ResolveUSDTNetwork(firstNonEmpty(req.Network, c.cfg.USDTNetwork, "TRC20"))
		netCode := "TRC20"
		if ok {
			netCode = netInfo.Network
		}
		tokenSymbol := strings.ToUpper(firstNonEmpty(req.Currency, "USDT"))
		if tokenSymbol != "USDC" {
			tokenSymbol = "USDT"
		}

		if mode == "stc" {
			apiKey := firstNonEmpty(req.APIKey, c.cfg.USDTAPIKey)
			secretKey := firstNonEmpty(req.SecretKey, c.cfg.USDTSecretKey)
			if apiKey == "" || secretKey == "" {
				return UnifiedInvoiceDTO{}, &Error{Code: "MISSING_STC_CREDENTIALS", Message: "RPAY_USDT_API_KEY and RPAY_USDT_SECRET_KEY are required for STC mode"}
			}
			raw, err := c.doRequest(ctx, http.MethodPost, "/api/AddInvoice", nil, nil, map[string]string{
				"api_key":      apiKey,
				"secret_key":   secretKey,
				"amount":       fmt.Sprintf("%v", req.Amount),
				"request_id":   req.OrderID,
				"callback_url": req.CallbackURL,
				"network":      netCode,
				"token":        tokenSymbol,
				"description":  req.Description,
			}, req.Headers)
			if err != nil {
				return UnifiedInvoiceDTO{}, err
			}
			return normalizeUnifiedInvoiceMap(raw, "usdt"), nil
		}

		headers := map[string]string{}
		for k, v := range req.Headers {
			headers[k] = v
		}
		if apiKey := firstNonEmpty(req.APIKey, c.cfg.USDTAPIKey); apiKey != "" {
			headers["Authorization"] = "Bearer " + apiKey
		}
		if req.IdempotencyKey != "" {
			headers["Idempotency-Key"] = req.IdempotencyKey
		}

		raw, err := c.doRequest(ctx, http.MethodPost, "/api/public/usdt-invoices", nil, map[string]any{
			"amount":         req.Amount,
			"network":        netCode,
			"token":          tokenSymbol,
			"orderId":        req.OrderID,
			"description":    req.Description,
			"buyerName":      req.BuyerName,
			"buyerEmail":     req.BuyerEmail,
			"buyerPhone":     req.BuyerPhone,
			"callbackUrl":    req.CallbackURL,
			"returnUrl":      req.ReturnURL,
			"cancelUrl":      req.CancelURL,
			"expiresInHours": req.ExpiresInHours,
		}, nil, headers)
		if err != nil {
			return UnifiedInvoiceDTO{}, err
		}
		return normalizeUnifiedInvoiceMap(raw, "usdt"), nil
	}

	// Bank (VND) channel
	token := firstNonEmpty(req.Token, c.cfg.BankToken)
	if token == "" {
		return UnifiedInvoiceDTO{}, &Error{Code: "MISSING_BANK_TOKEN", Message: "Bank token is required (set RPAY_BANK_TOKEN or pass PayRequest.Token)"}
	}
	headers := map[string]string{}
	for k, v := range req.Headers {
		headers[k] = v
	}
	if req.IdempotencyKey != "" {
		headers["Idempotency-Key"] = req.IdempotencyKey
	}

	raw, err := c.doRequest(ctx, http.MethodPost, "/api/public/bank-invoices", nil, map[string]any{
		"token":          token,
		"amount":         req.Amount,
		"orderId":        req.OrderID,
		"description":    req.Description,
		"buyerName":      req.BuyerName,
		"buyerEmail":     req.BuyerEmail,
		"buyerPhone":     req.BuyerPhone,
		"callbackUrl":    req.CallbackURL,
		"returnUrl":      req.ReturnURL,
		"cancelUrl":      req.CancelURL,
		"expiresInHours": req.ExpiresInHours,
	}, nil, headers)
	if err != nil {
		return UnifiedInvoiceDTO{}, err
	}
	return normalizeUnifiedInvoiceMap(raw, "bank"), nil
}

// CreateInvoice is an alias for Pay.
func (c *Client) CreateInvoice(ctx context.Context, req PayRequest) (UnifiedInvoiceDTO, error) {
	return c.Pay(ctx, req)
}

// CheckOptions configures optional parameters for Check / Status.
type CheckOptions struct {
	Channel   string
	Mode      string
	Token     string
	APIKey    string
	SecretKey string
}

// Check queries the status of a Bank (`RPINV...`) or Crypto (`USINV...` / `USTX...`) invoice.
func (c *Client) Check(ctx context.Context, codeOrRef string, opts ...CheckOptions) (UnifiedInvoiceDTO, error) {
	ref := strings.TrimSpace(codeOrRef)
	if ref == "" {
		return UnifiedInvoiceDTO{}, &Error{Code: "MISSING_INVOICE_REF", Message: "invoice code or reference is required"}
	}
	var opt CheckOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	ch := DetectChannel(firstNonEmpty(opt.Channel, ref), "", "")
	if ch == "usdt" {
		mode := strings.ToLower(firstNonEmpty(opt.Mode, c.cfg.USDTMode, "public"))
		if mode == "stc" {
			apiKey := firstNonEmpty(opt.APIKey, c.cfg.USDTAPIKey)
			secretKey := firstNonEmpty(opt.SecretKey, c.cfg.USDTSecretKey)
			raw, err := c.doRequest(ctx, http.MethodPost, "/api/GetInvoiceStatus", nil, nil, map[string]string{
				"api_key":    apiKey,
				"secret_key": secretKey,
				"trans_id":   ref,
			}, nil)
			if err != nil {
				return UnifiedInvoiceDTO{}, err
			}
			return normalizeUnifiedInvoiceMap(raw, "usdt"), nil
		}
		raw, err := c.doRequest(ctx, http.MethodGet, "/api/public/usdt-invoices/"+url.PathEscape(ref), nil, nil, nil, nil)
		if err != nil {
			return UnifiedInvoiceDTO{}, err
		}
		return normalizeUnifiedInvoiceMap(raw, "usdt"), nil
	}

	token := firstNonEmpty(opt.Token, c.cfg.BankToken)
	raw, err := c.doRequest(ctx, http.MethodGet, "/api/public/bank-invoices/"+url.PathEscape(ref), map[string]string{
		"token": token,
	}, nil, nil, nil)
	if err != nil {
		return UnifiedInvoiceDTO{}, err
	}
	return normalizeUnifiedInvoiceMap(raw, "bank"), nil
}

// Status is an alias for Check.
func (c *Client) Status(ctx context.Context, codeOrRef string, opts ...CheckOptions) (UnifiedInvoiceDTO, error) {
	return c.Check(ctx, codeOrRef, opts...)
}

// LSGDOptions configures parameters for fetching transaction history (LSGD) across Bank and/or USDT.
type LSGDOptions struct {
	Channel       string // "bank", "usdt", or "all"
	BankCode      string
	AccountNumber string
	Password      string
	Token         string
	APIKey        string
	SecretKey     string
	Status        string
	Network       string
	Limit         int
}

// LSGDResponse holds normalized transactions from Bank and/or USDT.
type LSGDResponse struct {
	Status       string                  `json:"status"`
	Channel      string                  `json:"channel"`
	Total        int                     `json:"total"`
	Transactions []UnifiedTransactionDTO `json:"transactions"`
}

// LSGD fetches normalized transaction history (Lịch sử giao dịch) from Bank, USDT, or both ("all").
func (c *Client) LSGD(ctx context.Context, opts ...LSGDOptions) (LSGDResponse, error) {
	var opt LSGDOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	ch := strings.ToLower(firstNonEmpty(opt.Channel, "all"))
	if opt.Channel == "" {
		hasBank := firstNonEmpty(opt.Token, c.cfg.BankToken) != ""
		hasUSDT := firstNonEmpty(opt.APIKey, c.cfg.USDTAPIKey) != "" && firstNonEmpty(opt.SecretKey, c.cfg.USDTSecretKey) != ""
		if hasBank && hasUSDT {
			ch = "all"
		} else if hasUSDT {
			ch = "usdt"
		} else {
			ch = "bank"
		}
	}

	var results []UnifiedTransactionDTO

	if ch == "bank" || ch == "all" {
		bankInfo, ok := ResolveBank(firstNonEmpty(opt.BankCode, c.cfg.BankCode, "ACB"))
		if !ok {
			return LSGDResponse{}, &Error{Code: "UNSUPPORTED_BANK", Message: "Unsupported bank code"}
		}
		token := firstNonEmpty(opt.Token, c.cfg.BankToken)
		acc := firstNonEmpty(opt.AccountNumber, c.cfg.AccountNumber)
		pwd := firstNonEmpty(opt.Password, c.cfg.Password)

		var raw map[string]any
		var err error
		if acc != "" && pwd != "" && token != "" {
			raw, err = c.doRequest(ctx, http.MethodPost, "/api/pos/transactions", nil, map[string]any{
				"provider":      bankInfo.Code,
				"accountNumber": acc,
				"password":      pwd,
				"token":         token,
			}, nil, nil)
		} else if token != "" {
			raw, err = c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/historyapi%sv2/%s", bankInfo.APISlug, url.PathEscape(token)), nil, nil, nil, nil)
		} else if ch == "bank" {
			return LSGDResponse{}, &Error{Code: "MISSING_BANK_TOKEN", Message: "Bank token is required for Bank LSGD"}
		}

		if err != nil && ch == "bank" {
			return LSGDResponse{}, err
		}
		if raw != nil {
			if list, ok := raw["transactions"].([]any); ok {
				for _, item := range list {
					if m, ok := item.(map[string]any); ok {
						results = append(results, normalizeUnifiedTxMap(m, "bank", bankInfo.Code, acc))
					}
				}
			}
		}
	}

	if ch == "usdt" || ch == "all" {
		apiKey := firstNonEmpty(opt.APIKey, c.cfg.USDTAPIKey)
		secretKey := firstNonEmpty(opt.SecretKey, c.cfg.USDTSecretKey)
		if apiKey != "" && secretKey != "" {
			limitStr := ""
			if opt.Limit > 0 {
				limitStr = fmt.Sprintf("%d", opt.Limit)
			}
			raw, err := c.doRequest(ctx, http.MethodPost, "/api/GetInvoiceHistory", nil, nil, map[string]string{
				"api_key":    apiKey,
				"secret_key": secretKey,
				"status":     opt.Status,
				"network":    opt.Network,
				"limit":      limitStr,
			}, nil)
			if err != nil && ch == "usdt" {
				return LSGDResponse{}, err
			}
			if raw != nil {
				if list, ok := raw["data"].([]any); ok {
					for _, item := range list {
						if m, ok := item.(map[string]any); ok {
							results = append(results, normalizeUnifiedTxMap(m, "usdt", "", ""))
						}
					}
				}
			}
		} else if ch == "usdt" {
			return LSGDResponse{}, &Error{Code: "MISSING_STC_CREDENTIALS", Message: "USDT API key & secret key are required for USDT LSGD"}
		}
	}

	return LSGDResponse{
		Status:       "success",
		Channel:      ch,
		Total:        len(results),
		Transactions: results,
	}, nil
}

// QRRequest configures VietQR or Crypto QR generation.
type QRRequest struct {
	Channel        string
	BankCode       string
	BankBIN        string
	AccountNo      string
	AccountName    string
	Amount         float64
	AddInfo        string
	Template       string
	Network        string
	WalletAddress  string
	Currency       string
	Format         string
}

// QR generates a VietQR payload (for Bank) or Wallet QR metadata (for USDT/USDC).
func (c *Client) QR(ctx context.Context, req QRRequest) (map[string]any, error) {
	ch := DetectChannel(req.Channel, req.Currency, req.Network)
	if ch == "usdt" || (req.WalletAddress != "" && req.BankCode == "" && req.BankBIN == "") {
		netInfo, ok := ResolveUSDTNetwork(firstNonEmpty(req.Network, c.cfg.USDTNetwork, "TRC20"))
		netCode := "TRC20"
		if ok {
			netCode = netInfo.Network
		}
		return map[string]any{
			"channel":       "usdt",
			"network":       netCode,
			"currency":      strings.ToUpper(firstNonEmpty(req.Currency, "USDT")),
			"walletAddress": req.WalletAddress,
			"amount":        req.Amount,
			"qrContent":     req.WalletAddress,
		}, nil
	}

	bankBIN := strings.TrimSpace(req.BankBIN)
	if bankBIN == "" {
		if info, ok := ResolveBank(firstNonEmpty(req.BankCode, c.cfg.BankCode, "ACB")); ok {
			bankBIN = info.BIN
		}
	}
	accNo := firstNonEmpty(req.AccountNo, c.cfg.AccountNumber)
	query := map[string]string{
		"bankBin":     bankBIN,
		"accountNo":   accNo,
		"accountName": req.AccountName,
		"addInfo":     req.AddInfo,
		"template":    firstNonEmpty(req.Template, "compact2"),
		"format":      firstNonEmpty(req.Format, "json"),
	}
	if req.Amount > 0 {
		query["amount"] = fmt.Sprintf("%.0f", req.Amount)
	}
	return c.doRequest(ctx, http.MethodGet, "/api/qr/vietqr", query, nil, nil, nil)
}

// RouteRequest configures the rpay.Route(...) call (matching the Hero Card SDK snippet).
type RouteRequest struct {
	Provider      string
	Security      string
	Coalesced     string
	WebhookSecret string
	BankCode      string
	AccountNumber string
	Password      string
	Token         string
	Ping          bool
}

// RouteResponse represents the routing & health summary returned by Route(...).
type RouteResponse struct {
	Success        bool              `json:"success"`
	Provider       string            `json:"provider"`
	Security       string            `json:"security"`
	Coalesced      string            `json:"coalesced"`
	SocketLive     bool              `json:"socketLive"`
	SupportedBanks int               `json:"supportedBanks"`
	SupportedUSDT  int               `json:"supportedUsdtNetworks"`
	Domain         string            `json:"domain"`
	Signature      string            `json:"signature,omitempty"`
	Endpoints      map[string]string `json:"endpoints"`
	LiveData       map[string]any    `json:"liveData,omitempty"`
}

// Route auto-routes bank/webhook metadata and optionally pings live LSGD endpoints.
func (c *Client) Route(ctx context.Context, req RouteRequest) (RouteResponse, error) {
	provider := firstNonEmpty(req.Provider, "9ROUTER")
	security := firstNonEmpty(req.Security, "HMAC_256")
	coalesced := firstNonEmpty(req.Coalesced, "498.310 req")
	secret := firstNonEmpty(req.WebhookSecret, c.cfg.WebhookSecret)

	sig := ""
	if secret != "" && strings.Contains(strings.ToUpper(security), "HMAC") {
		b, _ := json.Marshal(map[string]string{
			"provider":  provider,
			"security":  security,
			"coalesced": coalesced,
		})
		sig = SignRawBody(b, secret)
	}

	var liveData map[string]any
	if req.Ping {
		lsgdRes, err := c.LSGD(ctx, LSGDOptions{
			Channel:       "bank",
			BankCode:      req.BankCode,
			AccountNumber: req.AccountNumber,
			Password:      req.Password,
			Token:         req.Token,
		})
		if err != nil {
			return RouteResponse{}, err
		}
		liveData = map[string]any{"total": lsgdRes.Total}
	}

	return RouteResponse{
		Success:        true,
		Provider:       provider,
		Security:       security,
		Coalesced:      coalesced,
		SocketLive:     true,
		SupportedBanks: len(Banks()),
		SupportedUSDT:  len(USDTNetworks()),
		Domain:         firstNonEmpty(c.siteDomain, c.baseURL),
		Signature:      sig,
		Endpoints: map[string]string{
			"bankInvoice": "/api/public/bank-invoices",
			"usdtInvoice": "/api/public/usdt-invoices",
			"stcInvoice":  "/api/AddInvoice",
			"bankPos":     "/api/pos/transactions",
			"usdtHistory": "/api/GetInvoiceHistory",
			"vietqr":      "/api/qr/vietqr",
		},
		LiveData: liveData,
	}, nil
}

// BuildV3GetURL constructs a safe V3 GET URL, automatically encoding '#' in passwords to '%23'.
func (c *Client) BuildV3GetURL(bankCode, password, accountNumber, token string) (string, error) {
	info, ok := ResolveBank(firstNonEmpty(bankCode, c.cfg.BankCode))
	if !ok {
		return "", &Error{Code: "UNSUPPORTED_BANK", Message: "Unsupported bank code: " + bankCode}
	}
	pwd := firstNonEmpty(password, c.cfg.Password)
	acc := firstNonEmpty(accountNumber, c.cfg.AccountNumber)
	tok := firstNonEmpty(token, c.cfg.BankToken)
	return fmt.Sprintf(
		"%s/historyapi%sv3/%s/%s/%s",
		c.baseURL,
		info.APISlug,
		url.PathEscape(pwd),
		url.PathEscape(acc),
		url.PathEscape(tok),
	), nil
}

// VerifyWebhook verifies an incoming RPAY webhook using the client's configured WebhookSecret (or an override).
func (c *Client) VerifyWebhook(in VerifyWebhookInput) UnifiedWebhookEvent {
	if in.Secret == "" {
		in.Secret = c.cfg.WebhookSecret
	}
	return VerifyWebhook(in)
}
