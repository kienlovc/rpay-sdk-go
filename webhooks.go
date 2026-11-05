package rpay

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// UnifiedWebhookEvent represents a verified, normalized webhook event from RPAY
// (compatible with Bank Invoice `bank.invoice.paid`, USDT Store `crypto.invoice.paid`, and STC `bank.transactions`).
type UnifiedWebhookEvent struct {
	Verified         bool              `json:"verified"`
	Reason           string            `json:"reason"`
	Scheme           string            `json:"scheme"`
	Channel          string            `json:"channel"` // "bank" or "usdt"
	Event            string            `json:"event"`
	InvoiceCode      string            `json:"invoiceCode"`
	OrderID          string            `json:"orderId"`
	Status           string            `json:"status"`
	Amount           float64           `json:"amount"`
	PaidAmount       float64           `json:"paidAmount"`
	Currency         string            `json:"currency"` // "VND", "USDT", "USDC"
	NetworkOrBank    string            `json:"networkOrBank"`
	TransactionID    string            `json:"transactionId"`
	TransferContent  string            `json:"transferContent"`
	RecipientAddress string            `json:"recipientAddress"`
	SenderAddress    string            `json:"senderAddress"`
	CanonicalPayload string            `json:"canonicalPayload,omitempty"`
	Payload          map[string]any    `json:"payload"`
}

// BuildCanonicalPayloadString builds the sorted `key=value&...` string (excluding `signature`)
// used by RPAY Bank Invoice Webhooks and USDT Crypto Store Webhooks.
func BuildCanonicalPayloadString(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	keys := make([]string, 0, len(payload))
	for k := range payload {
		if k == "signature" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		var val string
		switch v := payload[k].(type) {
		case float64:
			val = strconv.FormatFloat(v, 'f', -1, 64)
		case float32:
			val = strconv.FormatFloat(float64(v), 'f', -1, 32)
		case nil:
			val = ""
		default:
			val = strings.TrimSpace(fmt.Sprintf("%v", v))
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, val))
	}
	return strings.Join(parts, "&")
}

// HMACSHA256Hex computes the lowercase hex HMAC-SHA256 digest of message using secret.
func HMACSHA256Hex(secret string, message []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}

// SafeCompareSignature performs a constant-time comparison of two signature strings.
func SafeCompareSignature(expected, received string) bool {
	a := strings.ToLower(strings.TrimSpace(expected))
	b := strings.ToLower(strings.TrimSpace(received))
	if a == "" || b == "" || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// SignCanonicalPayload signs a map payload using RPAY's sorted `key=value&...` HMAC-SHA256 specification.
func SignCanonicalPayload(payload map[string]any, secret string) (signature string, canonical string) {
	canonical = BuildCanonicalPayloadString(payload)
	signature = HMACSHA256Hex(secret, []byte(canonical))
	return signature, canonical
}

// SignRawBody signs raw HTTP request body bytes in `X-RPay-Body-Signature` format (`sha256=<hex>`).
func SignRawBody(rawBody []byte, secret string) string {
	return "sha256=" + HMACSHA256Hex(secret, rawBody)
}

// VerifyWebhookInput holds input parameters for verifying an incoming RPAY webhook.
type VerifyWebhookInput struct {
	Payload map[string]any
	RawBody []byte
	Headers map[string]string
	Secret  string
}

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	case string:
		clean := strings.ReplaceAll(strings.TrimSpace(val), ",", "")
		f, _ := strconv.ParseFloat(clean, 64)
		return f
	default:
		return 0
	}
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

func normalizeWebhookMap(base UnifiedWebhookEvent) UnifiedWebhookEvent {
	p := base.Payload
	if p == nil {
		return base
	}
	eventName := base.Event
	if eventName == "" {
		eventName = toString(p["event"])
	}

	if strings.HasPrefix(eventName, "crypto.") || p["trans_id"] != nil || p["invoice_code"] != nil || p["network"] != nil {
		base.Channel = "usdt"
		if eventName == "" {
			eventName = "crypto.invoice.paid"
		}
		base.Event = eventName
		base.InvoiceCode = firstNonEmpty(toString(p["invoice_code"]), toString(p["invoiceCode"]), toString(p["trans_id"]))
		base.OrderID = firstNonEmpty(toString(p["request_id"]), toString(p["orderId"]), toString(p["order_id"]))
		base.Status = "paid"
		base.Amount = toFloat64(firstNonNil(p["amount"], p["amountUsdt"]))
		base.PaidAmount = toFloat64(firstNonNil(p["received"], p["receivedUsdt"], p["amount"]))
		base.Currency = strings.ToUpper(firstNonEmpty(toString(p["token"]), "USDT"))
		base.NetworkOrBank = strings.ToUpper(firstNonEmpty(toString(p["network"]), "TRC20"))
		base.TransactionID = firstNonEmpty(toString(p["transaction_id"]), toString(p["txHash"]), toString(p["trans_id"]))
		base.TransferContent = base.OrderID
		base.RecipientAddress = firstNonEmpty(toString(p["to_address"]), toString(p["address"]))
		base.SenderAddress = toString(p["from_address"])
		return base
	}

	if eventName == "bank.transactions" || p["transactions"] != nil {
		base.Channel = "bank"
		base.Event = "bank.transactions"
		base.Status = "paid"
		base.Currency = "VND"
		if base.NetworkOrBank == "" {
			base.NetworkOrBank = firstNonEmpty(toString(p["bank"]), "BANK")
		}
		if base.RecipientAddress == "" {
			base.RecipientAddress = toString(p["accountNumber"])
		}
		return base
	}

	base.Channel = "bank"
	if eventName == "" {
		eventName = "bank.invoice.paid"
	}
	base.Event = eventName
	base.InvoiceCode = firstNonEmpty(toString(p["invoiceCode"]), toString(p["invoice_code"]))
	base.OrderID = firstNonEmpty(toString(p["orderId"]), toString(p["order_id"]))
	base.Status = "paid"
	base.Amount = toFloat64(firstNonNil(p["amountVnd"], p["amount"]))
	base.PaidAmount = toFloat64(firstNonNil(p["paidAmountVnd"], p["amountVnd"], p["amount"]))
	base.Currency = "VND"
	base.NetworkOrBank = firstNonEmpty(toString(p["bankCode"]), toString(p["bankKey"]), "BANK")
	base.TransactionID = firstNonEmpty(toString(p["transactionId"]), toString(p["matchedTransactionId"]))
	base.TransferContent = toString(p["transferContent"])
	base.RecipientAddress = toString(p["accountNumber"])
	return base
}

// VerifyWebhook auto-detects and verifies any RPAY webhook (`bank.invoice.paid`, `crypto.invoice.paid`, `bank.transactions`).
func VerifyWebhook(in VerifyWebhookInput) UnifiedWebhookEvent {
	hdrs := make(map[string]string, len(in.Headers))
	for k, v := range in.Headers {
		hdrs[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}

	parsed := in.Payload
	if parsed == nil && len(in.RawBody) > 0 {
		_ = json.Unmarshal(in.RawBody, &parsed)
	}

	secret := strings.TrimSpace(in.Secret)
	if secret == "" {
		return normalizeWebhookMap(UnifiedWebhookEvent{
			Verified: false,
			Reason:   "MISSING_WEBHOOK_SECRET",
			Scheme:   "none",
			Event:    firstNonEmpty(hdrs["x-rpay-event"], hdrs["x-stc-event"], "bank.invoice.paid"),
			Payload:  parsed,
		})
	}

	// 1. STC / Bank Transaction Webhook (`X-STC-HMAC-SHA256` / `X-STC-Signature`)
	if hdrs["x-stc-hmac-sha256"] != "" || hdrs["x-stc-signature"] != "" || hdrs["x-stc-event"] != "" {
		raw := in.RawBody
		if len(raw) == 0 && parsed != nil {
			raw, _ = json.Marshal(parsed)
		}
		if stcHmac := hdrs["x-stc-hmac-sha256"]; stcHmac != "" {
			expected := HMACSHA256Hex(secret, raw)
			ok := SafeCompareSignature(expected, stcHmac)
			reason := "OK"
			if !ok {
				reason = "STC_HMAC_MISMATCH"
			}
			return normalizeWebhookMap(UnifiedWebhookEvent{
				Verified:         ok,
				Reason:           reason,
				Scheme:           "x-stc-hmac-sha256",
				Event:            firstNonEmpty(hdrs["x-stc-event"], "bank.transactions"),
				NetworkOrBank:    hdrs["x-stc-bank"],
				RecipientAddress: hdrs["x-stc-account"],
				Payload:          parsed,
			})
		}
		plainSig := firstNonEmpty(hdrs["x-stc-signature"], hdrs["signature"])
		ok := SafeCompareSignature(secret, plainSig)
		reason := "OK"
		if !ok {
			reason = "STC_SIGNATURE_KEY_MISMATCH"
		}
		return normalizeWebhookMap(UnifiedWebhookEvent{
			Verified:         ok,
			Reason:           reason,
			Scheme:           "x-stc-signature",
			Event:            firstNonEmpty(hdrs["x-stc-event"], "bank.transactions"),
			NetworkOrBank:    hdrs["x-stc-bank"],
			RecipientAddress: hdrs["x-stc-account"],
			Payload:          parsed,
		})
	}

	// 2. Raw Body Signature (`X-RPay-Body-Signature`: `sha256=<hex>`)
	if bodySig := hdrs["x-rpay-body-signature"]; bodySig != "" && len(in.RawBody) > 0 {
		expected := SignRawBody(in.RawBody, secret)
		normReceived := bodySig
		if !strings.HasPrefix(normReceived, "sha256=") {
			normReceived = "sha256=" + normReceived
		}
		ok := SafeCompareSignature(expected, normReceived)
		reason := "OK"
		if !ok {
			reason = "BODY_SIGNATURE_MISMATCH"
		}
		return normalizeWebhookMap(UnifiedWebhookEvent{
			Verified: ok,
			Reason:   reason,
			Scheme:   "x-rpay-body-signature",
			Event:    firstNonEmpty(hdrs["x-rpay-event"], "bank.invoice.paid"),
			Payload:  parsed,
		})
	}

	// 3. Canonical Sorted Key-Value Signature (`X-RPay-Signature` or `payload.signature`)
	if parsed != nil {
		cand := firstNonEmpty(hdrs["x-rpay-signature"], toString(parsed["signature"]))
		if cand == "" {
			return normalizeWebhookMap(UnifiedWebhookEvent{
				Verified: false,
				Reason:   "MISSING_SIGNATURE",
				Scheme:   "canonical-hmac-sha256",
				Event:    firstNonEmpty(hdrs["x-rpay-event"], toString(parsed["event"]), "bank.invoice.paid"),
				Payload:  parsed,
			})
		}
		expected, canonical := SignCanonicalPayload(parsed, secret)
		ok := SafeCompareSignature(expected, cand)
		reason := "OK"
		if !ok {
			reason = "CANONICAL_SIGNATURE_MISMATCH"
		}
		return normalizeWebhookMap(UnifiedWebhookEvent{
			Verified:         ok,
			Reason:           reason,
			Scheme:           "canonical-hmac-sha256",
			CanonicalPayload: canonical,
			Event:            firstNonEmpty(hdrs["x-rpay-event"], toString(parsed["event"]), "bank.invoice.paid"),
			Payload:          parsed,
		})
	}

	return UnifiedWebhookEvent{
		Verified: false,
		Reason:   "EMPTY_PAYLOAD",
		Scheme:   "none",
	}
}

// WebhookHTTPHandler returns a plug-and-play `http.HandlerFunc` that verifies RPAY webhooks
// and invokes `onPaid` for verified payment events.
func (c *Client) WebhookHTTPHandler(onPaid func(event UnifiedWebhookEvent) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawBody, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "code": "INVALID_BODY"})
			return
		}
		hdrs := make(map[string]string, len(r.Header))
		for k, vals := range r.Header {
			if len(vals) > 0 {
				hdrs[strings.ToLower(k)] = vals[0]
			}
		}
		ev := c.VerifyWebhook(VerifyWebhookInput{
			RawBody: rawBody,
			Headers: hdrs,
		})
		if !ev.Verified {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"code":    ev.Reason,
				"message": "Chữ ký Webhook RPAY không hợp lệ.",
			})
			return
		}
		if onPaid != nil {
			if err := onPaid(ev); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "code": "HANDLER_ERROR"})
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":     true,
			"received":    true,
			"channel":     ev.Channel,
			"event":       ev.Event,
			"invoiceCode": ev.InvoiceCode,
			"orderId":     ev.OrderID,
		})
	}
}
