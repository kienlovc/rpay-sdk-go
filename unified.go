package rpay

import (
	"strings"
	"unicode"
)

// UnifiedInvoiceDTO represents a normalized invoice compatible with BOTH Bank (VND) and Crypto (USDT/USDC).
type UnifiedInvoiceDTO struct {
	Channel          string         `json:"channel"` // "bank" or "usdt"
	InvoiceID        string         `json:"invoiceId"`
	InvoiceCode      string         `json:"invoiceCode"`
	TxCode           string         `json:"txCode,omitempty"`
	OrderID          string         `json:"orderId"`
	Status           string         `json:"status"` // "pending", "paid", "expired", "cancelled"
	RawStatus        string         `json:"rawStatus"`
	IsPaid           bool           `json:"isPaid"`
	Amount           float64        `json:"amount"`
	PaidAmount       float64        `json:"paidAmount"`
	Currency         string         `json:"currency"` // "VND", "USDT", "USDC"
	NetworkOrBank    string         `json:"networkOrBank"`
	BankBIN          string         `json:"bankBin,omitempty"`
	RecipientAddress string         `json:"recipientAddress"`
	RecipientName    string         `json:"recipientName"`
	TransferContent  string         `json:"transferContent"`
	Description      string         `json:"description"`
	PaymentURL       string         `json:"paymentUrl"`
	QRContent        string         `json:"qrContent"`
	QRImageURL       string         `json:"qrImageUrl"`
	QRDataURL        string         `json:"qrDataUrl,omitempty"`
	CancelToken      string         `json:"cancelToken,omitempty"`
	TransactionID    string         `json:"transactionId,omitempty"`
	RemainingSeconds int            `json:"remainingSeconds"`
	Raw              map[string]any `json:"raw"`
}

// UnifiedTransactionDTO represents a normalized transaction item (LSGD) compatible with BOTH Bank and USDT/USDC.
type UnifiedTransactionDTO struct {
	Channel          string         `json:"channel"` // "bank" or "usdt"
	TransactionID    string         `json:"transactionID"`
	InvoiceCode      string         `json:"invoiceCode,omitempty"`
	OrderID          string         `json:"orderId,omitempty"`
	Type             string         `json:"type"`   // "IN" or "OUT"
	Status           string         `json:"status"` // "completed", "pending", "expired", "cancelled"
	Amount           float64        `json:"amount"`
	Currency         string         `json:"currency"`
	NetworkOrBank    string         `json:"networkOrBank"`
	RecipientAddress string         `json:"recipientAddress"`
	SenderAddress    string         `json:"senderAddress,omitempty"`
	Description      string         `json:"description"`
	TransactionDate  string         `json:"transactionDate"`
	TxHash           string         `json:"txHash,omitempty"`
	Raw              map[string]any `json:"raw"`
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

// NormalizeTransferText strips non-alphanumeric separators and uppercases text for resilient bank memo matching.
func NormalizeTransferText(value string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// DetectChannel determines whether an invoice code or parameter set targets "bank" or "usdt".
func DetectChannel(codeOrChannel string, currency string, network string) string {
	ch := strings.ToLower(strings.TrimSpace(codeOrChannel))
	if ch == "usdt" || ch == "usdc" || ch == "crypto" {
		return "usdt"
	}
	if ch == "bank" || ch == "vnd" || ch == "vietqr" {
		return "bank"
	}
	upperCode := strings.ToUpper(strings.TrimSpace(codeOrChannel))
	if strings.HasPrefix(upperCode, "USINV") || strings.HasPrefix(upperCode, "USTX") {
		return "usdt"
	}
	if strings.HasPrefix(upperCode, "RPINV") || strings.HasPrefix(upperCode, "RPAY") {
		return "bank"
	}
	curr := strings.ToUpper(strings.TrimSpace(currency))
	if curr == "USDT" || curr == "USDC" {
		return "usdt"
	}
	if network != "" {
		if _, ok := ResolveUSDTNetwork(network); ok {
			return "usdt"
		}
	}
	return "bank"
}

func normalizeStatus(rawStatus string) string {
	s := strings.ToLower(strings.TrimSpace(rawStatus))
	switch s {
	case "paid", "reconciled", "completed", "success", "underpaid_settled", "settled":
		return "paid"
	case "expired", "timeout":
		return "expired"
	case "cancelled", "canceled", "failed":
		return "cancelled"
	default:
		return "pending"
	}
}

func normalizeUnifiedInvoiceMap(raw map[string]any, hintChannel string) UnifiedInvoiceDTO {
	data := raw
	if nested, ok := raw["data"].(map[string]any); ok && nested != nil {
		data = nested
	}
	ch := hintChannel
	if ch == "" {
		if toString(data["currency"]) == "VND" || data["amountVnd"] != nil || data["beneficiary"] != nil {
			ch = "bank"
		} else {
			ch = "usdt"
		}
	}

	if ch == "bank" {
		status := normalizeStatus(toString(data["status"]))
		beneficiary, _ := data["beneficiary"].(map[string]any)
		qr, _ := data["qr"].(map[string]any)
		amt := toFloat64(firstNonNil(data["amountVnd"], data["amount"]))
		paid := toFloat64(data["paidAmountVnd"])
		if paid == 0 && status == "paid" {
			paid = amt
		}
		return UnifiedInvoiceDTO{
			Channel:          "bank",
			InvoiceID:        firstNonEmpty(toString(data["invoiceId"]), toString(data["id"]), toString(data["invoiceCode"])),
			InvoiceCode:      toString(data["invoiceCode"]),
			OrderID:          toString(data["orderId"]),
			Status:           status,
			RawStatus:        firstNonEmpty(toString(data["status"]), "pending"),
			IsPaid:           status == "paid",
			Amount:           amt,
			PaidAmount:       paid,
			Currency:         "VND",
			NetworkOrBank:    firstNonEmpty(toString(beneficiary["bankCode"]), toString(data["provider"]), "BANK"),
			BankBIN:          toString(beneficiary["bankBin"]),
			RecipientAddress: toString(beneficiary["accountNumber"]),
			RecipientName:    toString(beneficiary["accountHolder"]),
			TransferContent:  toString(data["transferContent"]),
			Description:      toString(data["description"]),
			PaymentURL:       firstNonEmpty(toString(data["paymentUrl"]), toString(data["checkoutUrl"])),
			QRContent:        toString(qr["content"]),
			QRImageURL:       toString(qr["imageUrl"]),
			QRDataURL:        toString(qr["dataUrl"]),
			CancelToken:      toString(data["cancelToken"]),
			TransactionID:    toString(data["matchedTransactionId"]),
			RemainingSeconds: int(toFloat64(data["remainingSeconds"])),
			Raw:              data,
		}
	}

	status := normalizeStatus(toString(data["status"]))
	invCode := firstNonEmpty(toString(data["invoiceCode"]), toString(data["invoice_code"]), toString(data["trans_id"]), toString(data["txCode"]))
	txCode := firstNonEmpty(toString(data["txCode"]), toString(data["trans_id"]), invCode)
	amt := toFloat64(firstNonNil(data["amountUsdt"], data["amount"]))
	paid := toFloat64(firstNonNil(data["receivedUsdt"], data["receivedAmountUsdt"], data["received"]))
	if paid == 0 && status == "paid" {
		paid = amt
	}
	return UnifiedInvoiceDTO{
		Channel:          "usdt",
		InvoiceID:        firstNonEmpty(toString(data["id"]), toString(data["invoiceId"]), txCode, invCode),
		InvoiceCode:      invCode,
		TxCode:           txCode,
		OrderID:          firstNonEmpty(toString(data["orderId"]), toString(data["request_id"])),
		Status:           status,
		RawStatus:        firstNonEmpty(toString(data["status"]), "waiting"),
		IsPaid:           status == "paid",
		Amount:           amt,
		PaidAmount:       paid,
		Currency:         strings.ToUpper(firstNonEmpty(toString(data["token"]), "USDT")),
		NetworkOrBank:    strings.ToUpper(firstNonEmpty(toString(data["network"]), "TRC20")),
		RecipientAddress: firstNonEmpty(toString(data["walletAddress"]), toString(data["address"])),
		RecipientName:    firstNonEmpty(toString(data["networkName"]), toString(data["network"]), "USDT Wallet"),
		TransferContent:  firstNonEmpty(toString(data["orderId"]), toString(data["request_id"]), invCode),
		Description:      toString(data["description"]),
		PaymentURL:       firstNonEmpty(toString(data["paymentUrl"]), toString(data["checkoutUrl"]), toString(data["url_payment"])),
		QRContent:        firstNonEmpty(toString(data["qrContent"]), toString(data["walletAddress"]), toString(data["address"])),
		QRImageURL:       firstNonEmpty(toString(data["qrImageUrl"]), toString(data["qrcode"]), toString(data["qr_code"])),
		QRDataURL:        firstNonEmpty(toString(data["qrDataUrl"]), toString(data["qrcode"]), toString(data["qr_code"])),
		TransactionID:    firstNonEmpty(toString(data["txHash"]), toString(data["transaction_id"])),
		RemainingSeconds: int(toFloat64(data["remainingSeconds"])),
		Raw:              data,
	}
}

func normalizeUnifiedTxMap(rawTx map[string]any, channel, bankCode, accountNumber string) UnifiedTransactionDTO {
	if channel == "bank" {
		txType := strings.ToUpper(firstNonEmpty(toString(rawTx["type"]), "IN"))
		if txType != "OUT" {
			txType = "IN"
		}
		return UnifiedTransactionDTO{
			Channel:          "bank",
			TransactionID:    firstNonEmpty(toString(rawTx["transactionID"]), toString(rawTx["id"])),
			Type:             txType,
			Status:           "completed",
			Amount:           toFloat64(rawTx["amount"]),
			Currency:         "VND",
			NetworkOrBank:    firstNonEmpty(toString(rawTx["bankCode"]), bankCode, "BANK"),
			RecipientAddress: firstNonEmpty(toString(rawTx["accountNumber"]), accountNumber),
			Description:      toString(rawTx["description"]),
			TransactionDate:  toString(rawTx["transactionDate"]),
			Raw:              rawTx,
		}
	}

	normStatus := normalizeStatus(firstNonEmpty(toString(rawTx["status"]), "completed"))
	if normStatus == "paid" {
		normStatus = "completed"
	}
	txID := firstNonEmpty(
		toString(rawTx["transaction_id"]),
		toString(rawTx["txHash"]),
		toString(rawTx["trans_id"]),
		toString(rawTx["invoice_code"]),
		toString(rawTx["invoiceCode"]),
	)
	return UnifiedTransactionDTO{
		Channel:          "usdt",
		TransactionID:    txID,
		InvoiceCode:      firstNonEmpty(toString(rawTx["invoice_code"]), toString(rawTx["invoiceCode"]), toString(rawTx["trans_id"])),
		OrderID:          firstNonEmpty(toString(rawTx["request_id"]), toString(rawTx["orderId"])),
		Type:             "IN",
		Status:           normStatus,
		Amount:           toFloat64(firstNonNil(rawTx["received"], rawTx["receivedUsdt"], rawTx["amount"], rawTx["amountUsdt"])),
		Currency:         strings.ToUpper(firstNonEmpty(toString(rawTx["token"]), "USDT")),
		NetworkOrBank:    strings.ToUpper(firstNonEmpty(toString(rawTx["network"]), "TRC20")),
		RecipientAddress: firstNonEmpty(toString(rawTx["address"]), toString(rawTx["walletAddress"])),
		SenderAddress:    firstNonEmpty(toString(rawTx["from_address"]), toString(rawTx["fromAddress"])),
		Description:      firstNonEmpty(toString(rawTx["description"]), toString(rawTx["request_id"]), toString(rawTx["orderId"])),
		TransactionDate:  firstNonEmpty(toString(rawTx["update_gettime"]), toString(rawTx["create_gettime"]), toString(rawTx["completedAt"])),
		TxHash:           firstNonEmpty(toString(rawTx["transaction_id"]), toString(rawTx["txHash"])),
		Raw:              rawTx,
	}
}

// MatchCriteria defines matching rules for idempotent incoming payment reconciliation across Bank & USDT.
type MatchCriteria struct {
	Channel            string
	OrderCode          string
	ExpectedAmount     float64
	ExactAmount        bool
	TxHash             string
	ProcessedIDs       map[string]bool
	MutateProcessedIDs bool
}

// MatchPayment filters unified transactions matching `OrderCode` and `ExpectedAmount`, skipping already-processed IDs.
func MatchPayment(transactions []UnifiedTransactionDTO, criteria MatchCriteria) []UnifiedTransactionDTO {
	filterCh := strings.ToLower(firstNonEmpty(criteria.Channel, "all"))
	needleRaw := strings.TrimSpace(criteria.OrderCode)
	needleUpper := strings.ToUpper(needleRaw)
	needleCompact := NormalizeTransferText(needleRaw)
	txHashNeedle := strings.ToLower(strings.TrimSpace(criteria.TxHash))

	var matches []UnifiedTransactionDTO
	for _, item := range transactions {
		if filterCh != "all" && item.Channel != filterCh {
			continue
		}
		id := firstNonEmpty(item.TransactionID, item.InvoiceCode)
		if id == "" || (criteria.ProcessedIDs != nil && criteria.ProcessedIDs[id]) {
			continue
		}
		if item.Type != "IN" || (item.Status != "completed" && item.Status != "paid") {
			continue
		}
		if criteria.ExpectedAmount > 0 {
			if criteria.ExactAmount && item.Amount != criteria.ExpectedAmount {
				continue
			}
			if !criteria.ExactAmount && item.Amount < criteria.ExpectedAmount {
				continue
			}
		}
		if txHashNeedle != "" && strings.ToLower(item.TxHash) != txHashNeedle {
			continue
		}
		if needleRaw != "" {
			directHit := strings.Contains(strings.ToUpper(item.Description), needleUpper) ||
				strings.EqualFold(item.OrderID, needleRaw) ||
				strings.EqualFold(item.InvoiceCode, needleRaw)
			compactHit := needleCompact != "" &&
				(strings.Contains(NormalizeTransferText(item.Description), needleCompact) ||
					NormalizeTransferText(item.OrderID) == needleCompact)
			if !directHit && !compactHit {
				continue
			}
		}
		matches = append(matches, item)
		if criteria.MutateProcessedIDs && criteria.ProcessedIDs != nil {
			criteria.ProcessedIDs[id] = true
		}
	}
	return matches
}
