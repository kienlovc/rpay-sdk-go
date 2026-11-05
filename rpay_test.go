package rpay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogsAndRoute(t *testing.T) {
	if len(Banks()) != 15 {
		t.Fatalf("expected 15 banks, got %d", len(Banks()))
	}
	if len(USDTNetworks()) != 18 {
		t.Fatalf("expected 18 USDT networks, got %d", len(USDTNetworks()))
	}

	b, ok := ResolveBank("vietcombank")
	if !ok || b.Code != "VCB" || b.BIN != "970436" {
		t.Fatalf("unexpected bank resolution: %+v", b)
	}

	net, ok := ResolveUSDTNetwork("bsc")
	if !ok || net.Network != "BEP20" {
		t.Fatalf("unexpected network resolution: %+v", net)
	}

	client := New(WithWebhookSecret("whsec_go_test"))
	routeRes, err := client.Route(context.Background(), RouteRequest{
		Provider:  "9ROUTER",
		Security:  "HMAC_256",
		Coalesced: "498.310 req",
	})
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if !routeRes.Success || routeRes.SupportedBanks != 15 || routeRes.SupportedUSDT != 18 || !strings.HasPrefix(routeRes.Signature, "sha256=") {
		t.Fatalf("unexpected RouteResponse: %+v", routeRes)
	}
}

func TestMultiDomainPayLSGDAndMatch(t *testing.T) {
	var seenDomains []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenDomains = append(seenDomains, r.Header.Get("X-Site-Domain"))
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/api/public/bank-invoices" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"invoiceId":       "inv_go_1",
					"invoiceCode":     "RPINV_GO_1",
					"orderId":         "DH1001",
					"status":          "pending",
					"amountVnd":       250000,
					"currency":        "VND",
					"transferContent": "RPAY DH1001",
					"paymentUrl":      "https://pay.shopcon.vn/pay/RPINV_GO_1",
					"beneficiary": map[string]any{
						"bankCode":      "ACB",
						"bankBin":       "970416",
						"accountNumber": "12345678",
						"accountHolder": "CONG TY GO",
					},
					"qr": map[string]any{
						"content":  "000201010212...",
						"imageUrl": "https://pay.shopcon.vn/api/qr/vietqr",
					},
				},
			})

		case r.URL.Path == "/api/public/usdt-invoices" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": map[string]any{
					"id":            "usdt_go_1",
					"invoiceCode":   "USINV_GO_1",
					"txCode":        "USTX_GO_1",
					"orderId":       "DH1001",
					"status":        "pending",
					"amountUsdt":    50,
					"token":         "USDT",
					"network":       "BEP20",
					"walletAddress": "0xAbCdEf1234567890",
					"paymentUrl":    "https://pay.shopcon.vn/pay/usdt/USINV_GO_1",
				},
			})

		case strings.HasPrefix(r.URL.Path, "/historyapiacbv2/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"transactions": []map[string]any{
					{
						"transactionID":   "BANK_GO_TX_1",
						"amount":          250000,
						"description":     "Chuyen khoan RPAY-DH1001",
						"transactionDate": "05/11/2026 21:00:00",
						"type":            "IN",
					},
				},
			})

		case r.URL.Path == "/api/GetInvoiceHistory":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "success",
				"data": []map[string]any{
					{
						"trans_id":       "USINV_GO_1",
						"request_id":     "DH1001",
						"amount":         50,
						"received":       50,
						"status":         "completed",
						"network":        "BEP20",
						"token":          "USDT",
						"address":        "0xAbCdEf1234567890",
						"transaction_id": "0xhash_go_999",
					},
				},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	baseClient := New(
		WithDomain(srv.URL),
		WithBankToken("tok_go_1", "ACB", "12345678", ""),
		WithUSDT("ukey_1", "usec_1", "BEP20", "public"),
	)
	tenantClient := baseClient.ForDomain(srv.URL)
	tenantClient.siteDomain = "pay.shopcon.vn"

	ctx := context.Background()

	bankInv, err := tenantClient.Pay(ctx, PayRequest{
		Amount:  250000,
		OrderID: "DH1001",
	})
	if err != nil {
		t.Fatalf("Bank Pay failed: %v", err)
	}
	if bankInv.Channel != "bank" || bankInv.InvoiceCode != "RPINV_GO_1" || bankInv.Amount != 250000 {
		t.Fatalf("unexpected bankInv: %+v", bankInv)
	}

	usdtInv, err := tenantClient.Pay(ctx, PayRequest{
		Channel: "usdt",
		Amount:  50,
		Network: "BSC",
		OrderID: "DH1001",
	})
	if err != nil {
		t.Fatalf("USDT Pay failed: %v", err)
	}
	if usdtInv.Channel != "usdt" || usdtInv.NetworkOrBank != "BEP20" || usdtInv.RecipientAddress != "0xAbCdEf1234567890" {
		t.Fatalf("unexpected usdtInv: %+v", usdtInv)
	}

	lsgd, err := tenantClient.LSGD(ctx, LSGDOptions{Channel: "all"})
	if err != nil {
		t.Fatalf("LSGD failed: %v", err)
	}
	if lsgd.Total != 2 {
		t.Fatalf("expected 2 unified transactions, got %d", lsgd.Total)
	}

	matched := MatchPayment(lsgd.Transactions, MatchCriteria{OrderCode: "DH1001"})
	if len(matched) != 2 {
		t.Fatalf("expected 2 matched payments, got %d", len(matched))
	}

	v3URL, err := tenantClient.BuildV3GetURL("VCB", "MatKhau#123", "0123456789", "tok_vcb")
	if err != nil || !strings.Contains(v3URL, "MatKhau%23123") {
		t.Fatalf("expected encoded # in V3 URL, got %s (err=%v)", v3URL, err)
	}

	if len(seenDomains) == 0 || seenDomains[0] != "pay.shopcon.vn" {
		t.Fatalf("expected X-Site-Domain header 'pay.shopcon.vn', got %v", seenDomains)
	}
}

func TestWebhookVerification(t *testing.T) {
	secret := "whsec_go_secret"
	payload := map[string]any{
		"eventId":     "evt_go_1",
		"event":       "bank.invoice.paid",
		"timestamp":   1760000000,
		"invoiceCode": "RPINV_GO_1",
		"orderId":     "DH1001",
		"status":      "paid",
		"amountVnd":   250000,
	}
	sig, _ := SignCanonicalPayload(payload, secret)
	raw, _ := json.Marshal(payload)

	res := VerifyWebhook(VerifyWebhookInput{
		RawBody: raw,
		Headers: map[string]string{"X-RPay-Signature": sig},
		Secret:  secret,
	})
	if !res.Verified || res.Scheme != "canonical-hmac-sha256" || res.Status != "paid" || res.OrderID != "DH1001" {
		t.Fatalf("expected valid canonical webhook verification, got %+v", res)
	}
}
