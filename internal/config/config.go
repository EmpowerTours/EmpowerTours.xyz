package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabasePath              string
	MonadRPCURL               string
	DeployerPrivateKey        string
	JWTSecret                 string
	MembershipNFTAddress      string
	PaymentsContractAddress   string
	Port                      string
	StripeSecretKey           string
	StripeWebhookSecret       string
	AppleIssuerID             string
	AppleKeyID                string
	ApplePrivateKey           string
	AppleBundleID             string
	GooglePlayCredentialsJSON string
	FacebookAppID             string
	FacebookAppSecret         string
	LicenseRegistryAddress    string
	PinataGateway             string
	MiniappURL                string
}

func Load() (*Config, error) {
	// Load .env file if it exists; ignore error if missing
	_ = godotenv.Load()

	cfg := &Config{
		DatabasePath:              getEnv("DATABASE_PATH", "./empowertours.db"),
		MonadRPCURL:               getEnv("MONAD_RPC_URL", "https://rpc.monad.xyz"),
		DeployerPrivateKey:        os.Getenv("DEPLOYER_PRIVATE_KEY"),
		JWTSecret:                 os.Getenv("JWT_SECRET"),
		MembershipNFTAddress:      os.Getenv("MEMBERSHIP_NFT_ADDRESS"),
		PaymentsContractAddress:   os.Getenv("PAYMENTS_CONTRACT_ADDRESS"),
		Port:                      getEnv("PORT", "8080"),
		StripeSecretKey:           os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:       os.Getenv("STRIPE_WEBHOOK_SECRET"),
		AppleIssuerID:             os.Getenv("APPLE_ISSUER_ID"),
		AppleKeyID:                os.Getenv("APPLE_KEY_ID"),
		ApplePrivateKey:           os.Getenv("APPLE_PRIVATE_KEY"),
		AppleBundleID:             os.Getenv("APPLE_BUNDLE_ID"),
		GooglePlayCredentialsJSON: os.Getenv("GOOGLE_PLAY_CREDENTIALS_JSON"),
		FacebookAppID:             os.Getenv("FACEBOOK_APP_ID"),
		FacebookAppSecret:         os.Getenv("FACEBOOK_APP_SECRET"),
		// The v3 LicenseRegistry is the catalog. It replaced the Envio indexer,
		// which was deleted on 2026-08-29 and now answers 404 — see
		// internal/music/chain.go for why that failure was invisible.
		LicenseRegistryAddress: getEnv("LICENSE_REGISTRY_ADDRESS", "0x42EbcD44C2295702130f0A641633c691bA5f9480"),
		// Gateway for ipfs:// URIs in token metadata. Defaulted to the same
		// dedicated gateway the catalog already served, so resolved URLs do not
		// change shape for existing clients.
		PinataGateway: getEnv("PINATA_GATEWAY", "https://harlequin-used-hare-224.mypinata.cloud/ipfs/"),
		// MiniappURL is the Farcaster mini app, used to cross-check that our
		// indexer endpoint hasn't drifted from the one the mini app uses.
		MiniappURL: getEnv("MINIAPP_URL", "https://fcempowertours-production-6551.up.railway.app"),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
