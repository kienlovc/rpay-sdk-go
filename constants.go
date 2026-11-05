package rpay

import (
	"slices"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the canonical RPAY API gateway URL.
	DefaultBaseURL = "https://api.rpay.vn"
	// DefaultTimeout is the recommended 60-second HTTP timeout for bank 3FA/OTP sessions.
	DefaultTimeout = 60 * time.Second
)

// BankSpec represents canonical metadata for one of the 15 supported Vietnamese bank/wallet gateways.
type BankSpec struct {
	Key            string   `json:"key"`
	Code           string   `json:"code"`
	ShortName      string   `json:"shortName"`
	FullName       string   `json:"fullName"`
	BIN            string   `json:"bin"`
	APISlug        string   `json:"apiSlug"`
	PrimarySlug    string   `json:"primarySlug"`
	Aliases        []string `json:"aliases"`
	Logo           string   `json:"logo"`
	V3UsesPhonePin bool     `json:"v3UsesPhonePin"`
}

// USDTNetworkSpec represents canonical metadata for one of the 18 supported USDT/USDC blockchain networks.
type USDTNetworkSpec struct {
	Network     string `json:"network"`
	Name        string `json:"name"`
	ChainKey    string `json:"chainKey"`
	Standard    string `json:"tokenStandard"`
	Decimals    int    `json:"decimals"`
	ExplorerURL string `json:"explorerBaseUrl"`
}

var canonicalBanks = []BankSpec{
	{Key: "vietcombank", Code: "VCB", ShortName: "Vietcombank", FullName: "Ngân hàng TMCP Ngoại thương Việt Nam", BIN: "970436", APISlug: "vcb", PrimarySlug: "vcb", Aliases: []string{"vcb", "vietcombank"}, Logo: "/ibankings/970436.png"},
	{Key: "acb", Code: "ACB", ShortName: "ACB", FullName: "Ngân hàng TMCP Á Châu", BIN: "970416", APISlug: "acb", PrimarySlug: "acb", Aliases: []string{"acb"}, Logo: "/ibankings/970416.png"},
	{Key: "mbbank", Code: "MB", ShortName: "MBBank", FullName: "Ngân hàng TMCP Quân đội", BIN: "970422", APISlug: "mb", PrimarySlug: "mb", Aliases: []string{"mb", "mbbank"}, Logo: "/ibankings/970422.png"},
	{Key: "techcombank", Code: "TCB", ShortName: "Techcombank", FullName: "Ngân hàng TMCP Kỹ thương Việt Nam", BIN: "970407", APISlug: "tcb", PrimarySlug: "tcb", Aliases: []string{"tcb", "techcombank"}, Logo: "/ibankings/970407.png"},
	{Key: "bidv", Code: "BIDV", ShortName: "BIDV", FullName: "Ngân hàng TMCP Đầu tư và Phát triển Việt Nam", BIN: "970418", APISlug: "bidv", PrimarySlug: "bidv", Aliases: []string{"bidv"}, Logo: "/ibankings/970418.png"},
	{Key: "vietinbank", Code: "ICB", ShortName: "VietinBank", FullName: "Ngân hàng TMCP Công thương Việt Nam", BIN: "970415", APISlug: "icb", PrimarySlug: "icb", Aliases: []string{"icb", "vietinbank", "ctg", "vtb", "viettinbank"}, Logo: "/ibankings/970415.png"},
	{Key: "vpbank", Code: "VPB", ShortName: "VPBank", FullName: "Ngân hàng TMCP Việt Nam Thịnh Vượng", BIN: "970432", APISlug: "vpb", PrimarySlug: "vpb", Aliases: []string{"vpb", "vpbank"}, Logo: "/ibankings/970432.png"},
	{Key: "tpbank", Code: "TPB", ShortName: "TPBank", FullName: "Ngân hàng TMCP Tiên Phong", BIN: "970423", APISlug: "tpb", PrimarySlug: "tpb", Aliases: []string{"tpb", "tpbank"}, Logo: "/ibankings/970423.png"},
	{Key: "agribank", Code: "VBA", ShortName: "Agribank", FullName: "Ngân hàng Nông nghiệp và PTNT Việt Nam", BIN: "970405", APISlug: "vba", PrimarySlug: "vba", Aliases: []string{"vba", "agribank"}, Logo: "/ibankings/970405.png"},
	{Key: "msb", Code: "MSB", ShortName: "MSB", FullName: "Ngân hàng TMCP Hàng Hải", BIN: "970426", APISlug: "msb", PrimarySlug: "msb", Aliases: []string{"msb", "maritimebank"}, Logo: "/ibankings/970426.png"},
	{Key: "ocb", Code: "OCB", ShortName: "OCB", FullName: "Ngân hàng TMCP Phương Đông", BIN: "970448", APISlug: "ocb", PrimarySlug: "ocb", Aliases: []string{"ocb", "oceanbank"}, Logo: "/ibankings/970448.png"},
	{Key: "seabank", Code: "SEAB", ShortName: "SeABank", FullName: "Ngân hàng TMCP Đông Nam Á", BIN: "970440", APISlug: "seab", PrimarySlug: "seabank", Aliases: []string{"seabank", "ssb", "seab", "sebank"}, Logo: "/ibankings/970440.png"},
	{Key: "eximbank", Code: "EIB", ShortName: "Eximbank", FullName: "Ngân hàng TMCP Xuất Nhập khẩu Việt Nam", BIN: "970431", APISlug: "eib", PrimarySlug: "eib", Aliases: []string{"eib", "eximbank", "exim"}, Logo: "/ibankings/970431.png"},
	{Key: "bvbank", Code: "BVB", ShortName: "BVBank", FullName: "Ngân hàng TMCP Bản Việt", BIN: "970454", APISlug: "bvb", PrimarySlug: "bvb", Aliases: []string{"bvb", "bvbank", "banviet", "vietcapitalbank"}, Logo: "/ibankings/970454.png"},
	{Key: "viettelpay", Code: "VTP", ShortName: "Viettel Money", FullName: "Tổng công ty Dịch vụ số Viettel", BIN: "971005", APISlug: "vtp", PrimarySlug: "vtm", Aliases: []string{"vtm", "vtp", "viettelmoney", "viettelpay", "viettel_money", "viettel-money"}, Logo: "/ibankings/971005.png", V3UsesPhonePin: true},
}

var canonicalUSDTNetworks = []USDTNetworkSpec{
	{Network: "TRC20", Name: "TRON (TRC-20)", ChainKey: "tron", Standard: "TRC-20", Decimals: 6, ExplorerURL: "https://tronscan.org"},
	{Network: "BEP20", Name: "BNB Smart Chain (BEP-20)", ChainKey: "bsc", Standard: "BEP-20", Decimals: 18, ExplorerURL: "https://bscscan.com"},
	{Network: "ERC20", Name: "Ethereum (ERC-20)", ChainKey: "ethereum", Standard: "ERC-20", Decimals: 6, ExplorerURL: "https://etherscan.io"},
	{Network: "POLYGON", Name: "Polygon POS", ChainKey: "polygon", Standard: "ERC-20 (Polygon)", Decimals: 6, ExplorerURL: "https://polygonscan.com"},
	{Network: "ARBITRUM", Name: "Arbitrum One", ChainKey: "arbitrum", Standard: "ERC-20 (Arbitrum)", Decimals: 6, ExplorerURL: "https://arbiscan.io"},
	{Network: "OPTIMISM", Name: "OP Mainnet (Optimism)", ChainKey: "optimism", Standard: "ERC-20 (Optimism)", Decimals: 6, ExplorerURL: "https://optimistic.etherscan.io"},
	{Network: "SOLANA", Name: "Solana (SPL)", ChainKey: "solana", Standard: "SPL", Decimals: 6, ExplorerURL: "https://solscan.io"},
	{Network: "TON", Name: "The Open Network (TON Jetton)", ChainKey: "ton", Standard: "Jetton", Decimals: 6, ExplorerURL: "https://tonviewer.com"},
	{Network: "BASE", Name: "Base Mainnet", ChainKey: "base", Standard: "ERC-20 (Base)", Decimals: 6, ExplorerURL: "https://basescan.org"},
	{Network: "AVALANCHE", Name: "Avalanche C-Chain", ChainKey: "avalanche", Standard: "ARC-20", Decimals: 6, ExplorerURL: "https://snowtrace.io"},
	{Network: "APTOS", Name: "Aptos Mainnet", ChainKey: "aptos", Standard: "Aptos Fungible Asset", Decimals: 6, ExplorerURL: "https://aptoscan.com"},
	{Network: "SUI", Name: "Sui Network", ChainKey: "sui", Standard: "Sui Coin", Decimals: 6, ExplorerURL: "https://suiscan.xyz/mainnet"},
	{Network: "OPBNB", Name: "opBNB Mainnet", ChainKey: "opbnb", Standard: "BEP-20 (opBNB)", Decimals: 18, ExplorerURL: "https://opbnbscan.com"},
	{Network: "CELO", Name: "Celo Mainnet", ChainKey: "celo", Standard: "ERC-20 (Celo)", Decimals: 6, ExplorerURL: "https://celoscan.io"},
	{Network: "SCROLL", Name: "Scroll Mainnet", ChainKey: "scroll", Standard: "ERC-20 (Scroll)", Decimals: 6, ExplorerURL: "https://scrollscan.com"},
	{Network: "KAVA", Name: "Kava EVM", ChainKey: "kava", Standard: "ERC-20 (Kava)", Decimals: 6, ExplorerURL: "https://kavascan.com"},
	{Network: "POLKADOT", Name: "Polkadot Asset Hub (Statemint)", ChainKey: "polkadot", Standard: "AssetHub #1984", Decimals: 6, ExplorerURL: "https://assethub-polkadot.subscan.io"},
	{Network: "OSMOSIS", Name: "Osmosis (Cosmos IBC)", ChainKey: "osmosis", Standard: "IBC Denom", Decimals: 6, ExplorerURL: "https://www.mintscan.io/osmosis"},
}

// Banks returns a defensive copy of all 15 supported Vietnamese bank gateways.
func Banks() []BankSpec {
	return slices.Clone(canonicalBanks)
}

// USDTNetworks returns a defensive copy of all 18 supported USDT/USDC blockchain networks.
func USDTNetworks() []USDTNetworkSpec {
	return slices.Clone(canonicalUSDTNetworks)
}

// ResolveBank resolves a bank specification by key, code, BIN, shortName, or alias.
func ResolveBank(identifier string) (BankSpec, bool) {
	norm := strings.ToLower(strings.TrimSpace(identifier))
	if norm == "" {
		return BankSpec{}, false
	}
	for _, b := range canonicalBanks {
		if b.Key == norm ||
			strings.EqualFold(b.Code, norm) ||
			b.BIN == norm ||
			strings.EqualFold(b.ShortName, norm) ||
			b.APISlug == norm ||
			b.PrimarySlug == norm ||
			slices.Contains(b.Aliases, norm) {
			return b, true
		}
	}
	return BankSpec{}, false
}

// ResolveUSDTNetwork resolves a canonical USDT/USDC network specification by code or alias.
func ResolveUSDTNetwork(rawNetwork string) (USDTNetworkSpec, bool) {
	norm := strings.ToUpper(strings.TrimSpace(rawNetwork))
	if norm == "" {
		return USDTNetworkSpec{}, false
	}
	aliasMap := map[string]string{
		"TRX":      "TRC20",
		"TRON":     "TRC20",
		"TRC-20":   "TRC20",
		"BSC":      "BEP20",
		"BNB":      "BEP20",
		"BEP-20":   "BEP20",
		"ETH":      "ERC20",
		"ETHEREUM": "ERC20",
		"ERC-20":   "ERC20",
		"MATIC":    "POLYGON",
		"ARB":      "ARBITRUM",
		"OP":       "OPTIMISM",
		"SOL":      "SOLANA",
		"AVAX":     "AVALANCHE",
		"DOT":      "POLKADOT",
		"OSMO":     "OSMOSIS",
	}
	if mapped, ok := aliasMap[norm]; ok {
		norm = mapped
	}
	for _, item := range canonicalUSDTNetworks {
		if item.Network == norm || strings.EqualFold(item.ChainKey, norm) {
			return item, true
		}
	}
	return USDTNetworkSpec{}, false
}
