# RPAY Golang SDK (`github.com/kienlovc/rpay-sdk-go`)

**Thư viện Golang chính thức cho hệ sinh thái RPAY Payment & Banking Gateway** — tích hợp đồng thời **15 ngân hàng Việt Nam (VietQR / Auto-Bank LSGD)** và **Crypto USDT/USDC (18 mạng lưới Blockchain)** trên **mọi tên miền (Multi-Domain / White-Label Child Site)**.

- **Zero External Dependencies**: Chỉ sử dụng thư viện chuẩn Go (`net/http`, `crypto/hmac`, `crypto/sha256`, `encoding/json`) — biên dịch nhanh, an toàn tuyệt đối.
- **Zero-Config `.env` Ready**: Tự động đọc biến môi trường `RPAY_*` ngay khi gọi `rpay.New()`.
- **Unified API (`Pay`, `Check`, `LSGD`, `QR`, `MatchPayment`)**: Dùng chung 1 struct kết quả [`UnifiedInvoiceDTO`](unified.go) và [`UnifiedTransactionDTO`](unified.go) cho cả Ngân hàng (VND) và Crypto (USDT/USDC).
- **Multi-Domain / White-Label**: Đổi domain động trên từng request qua `client.ForDomain("pay.shopcuaban.com")`.
- **Webhook Middleware chuẩn `net/http`**: Đi kèm [`WebhookHTTPHandler`](webhooks.go) xác thực `HMAC-SHA256` chống giả mạo.

---

## 1. Cài đặt

```bash
go get github.com/kienlovc/rpay-sdk-go
```

---

## 2. Cấu hình nhanh qua biến môi trường (`.env`)

```env
RPAY_DOMAIN=https://rpay.vn
RPAY_BANK_TOKEN=tok_live_xxxxxxxxxxxx
RPAY_BANK_CODE=ACB
RPAY_ACCOUNT_NUMBER=12345678
RPAY_USDT_API_KEY=usdt_key_xxxxxxxx
RPAY_USDT_SECRET_KEY=usdt_sec_xxxxxxxx
RPAY_USDT_NETWORK=TRC20
RPAY_WEBHOOK_SECRET=whsec_xxxxxxxxxxxx
```

---

## 3. Sử dụng siêu gọn (Unified Bank & USDT)

```go
package main

import (
	"context"
	"fmt"
	"log"

	rpay "github.com/kienlovc/rpay-sdk-go"
)

func main() {
	ctx := context.Background()

	// Tự động đọc cấu hình từ biến môi trường RPAY_*
	client := rpay.New()

	// 1. Tạo hóa đơn Ngân hàng (VietQR VND)
	bankInvoice, err := client.Pay(ctx, rpay.PayRequest{
		Amount:      150000,
		OrderID:     "DH1001",
		Description: "Thanh toan don hang DH1001",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Bank QR URL:", bankInvoice.QRImageURL)
	fmt.Println("Bank Checkout:", bankInvoice.PaymentURL)

	// 2. Tạo hóa đơn Crypto (USDT - TRC20 / BEP20 / POLYGON / SOL / TON...)
	usdtInvoice, err := client.Pay(ctx, rpay.PayRequest{
		Channel: "usdt",
		Amount:  25.5,
		Network: "TRC20",
		OrderID: "DH1001",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Ví nhận USDT:", usdtInvoice.RecipientAddress)
	fmt.Println("USDT Checkout:", usdtInvoice.PaymentURL)

	// 3. Kiểm tra trạng thái hóa đơn (tự nhận diện RPINV... hoặc USINV...)
	status, err := client.Check(ctx, bankInvoice.InvoiceCode)
	if err == nil && status.IsPaid {
		fmt.Println("Đơn hàng đã thanh toán thành công!")
	}
}
```

---

## 4. Dùng trên nhiều Domain (Multi-Domain / White-Label Child Site)

Khi bạn vận hành nhiều website hoặc đại lý White-Label (`pay.shopA.vn`, `pay.shopB.vn`), sử dụng `client.ForDomain(...)` để SDK tự động định tuyến URL và gắn header `X-Site-Domain`:

```go
shopA := client.ForDomain("pay.shop-a.vn")
shopB := client.ForDomain("pay.shop-b.vn", rpay.WithBankToken("tok_shop_b", "VCB", "0123456789", ""))

invA, _ := shopA.Pay(ctx, rpay.PayRequest{Amount: 99000, OrderID: "A_101"})
invB, _ := shopB.Pay(ctx, rpay.PayRequest{Channel: "usdt", Amount: 10, Network: "BEP20", OrderID: "B_202"})
```

---

## 5. Lấy lịch sử giao dịch (LSGD) & Tự động đối soát đơn hàng (`MatchPayment`)

```go
// Lấy gộp cả LSGD Ngân hàng + LSGD USDT trong 1 lệnh duy nhất
lsgd, err := client.LSGD(ctx, rpay.LSGDOptions{
	Channel: "all", // "bank" | "usdt" | "all"
})

// Tự động tìm giao dịch khớp mã đơn "DH1001" và số tiền >= 150.000đ
matched := rpay.MatchPayment(lsgd.Transactions, rpay.MatchCriteria{
	OrderCode:      "DH1001",
	ExpectedAmount: 150000,
})

if len(matched) > 0 {
	fmt.Println("Khớp giao dịch:", matched[0].TransactionID, matched[0].Amount, matched[0].Currency)
}
```

---

## 6. Xác thực Webhook `HMAC-SHA256` (Chuẩn `net/http`, Gin, Echo, Fiber)

### 6.1. Dùng trực tiếp `rpay.WebhookHTTPHandler` (`net/http` / Chi / Gorilla Mux)

```go
http.Handle("/api/rpay/webhook", rpay.WebhookHTTPHandler("", func(w http.ResponseWriter, r *http.Request, event rpay.NormalizedWebhookEvent) {
	if event.IsPaid {
		// Cộng tiền / kích hoạt đơn hàng theo event.OrderID và event.TransactionID
		fmt.Printf("Đã nhận %v %s cho đơn %s (Kênh: %s)\n", event.PaidAmount, event.Currency, event.OrderID, event.Channel)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"received":true}`))
}))
```

### 6.2. Dùng trong Gin / Echo / Fiber

```go
res := client.VerifyWebhook(rawBodyBytes, headersMap, "")
if !res.Valid {
	// Trả về HTTP 401
}
if res.NormalizedEvent.IsPaid {
	// Duyệt đơn hàng an toàn
}
```

---

## 7. Danh sách Ngân hàng & Mạng USDT hỗ trợ

- **15 Ngân hàng Việt Nam (`rpay.BankCatalog`)**: `ACB`, `VCB`, `MB`, `BIDV`, `CTG` (VietinBank), `TCB` (Techcombank), `TPB` (TPBank), `VPB` (VPBank), `STB` (Sacombank), `MSB`, `OCB`, `SHB`, `SEAB` (SeABank), `EXIM` (Eximbank), `TIMO`.
- **18 Mạng lưới Crypto (`rpay.USDTNetworks`)**: `TRC20`, `ERC20`, `BEP20`, `POLYGON`, `SOL`, `TON`, `ARB`, `OP`, `AVAXC`, `BASE`, `APTOS`, `SUI`, `NEAR`, `KAVA`, `CELO`, `STATEMINT`, `LIQUID`, `TEZOS`.

---

## 8. Xuất bản lên GitHub / `pkg.go.dev`

```bash
cd plugin/golang
go test -v ./...
git tag v1.0.0
git push origin v1.0.0
```
