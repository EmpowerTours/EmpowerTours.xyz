package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabasePath            string
	MonadRPCURL             string
	DeployerPrivateKey      string
	JWTSecret               string
	MembershipNFTAddress    string
	PaymentsContractAddress string
	Port                    string
	StripeSecretKey         string
	StripeWebhookSecret     string
	AppleIssuerID           string
	AppleKeyID              string
	ApplePrivateKey         string
	AppleBundleID           string
	GooglePlayCredentialsJSON string
	FacebookAppID            string
	FacebookAppSecret        string
	EnvioEndpoint            string
}

func Load() (*Config, error) {
	// Load .env file if it exists; ignore error if missing
	_ = godotenv.Load()

	cfg := &Config{
		DatabasePath:            getEnv("DATABASE_PATH", "./empowertours.db"),
		MonadRPCURL:             getEnv("MONAD_RPC_URL", "https://rpc.monad.xyz"),
		DeployerPrivateKey:      os.Getenv("DEPLOYER_PRIVATE_KEY"),
		JWTSecret:               os.Getenv("JWT_SECRET"),
		MembershipNFTAddress:    os.Getenv("MEMBERSHIP_NFT_ADDRESS"),
		PaymentsContractAddress: os.Getenv("PAYMENTS_CONTRACT_ADDRESS"),
		Port:                    getEnv("PORT", "8080"),
		StripeSecretKey:           os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:      os.Getenv("STRIPE_WEBHOOK_SECRET"),
		AppleIssuerID:            os.Getenv("APPLE_ISSUER_ID"),
		AppleKeyID:               os.Getenv("APPLE_KEY_ID"),
		ApplePrivateKey:          os.Getenv("APPLE_PRIVATE_KEY"),
		AppleBundleID:            os.Getenv("APPLE_BUNDLE_ID"),
		GooglePlayCredentialsJSON: os.Getenv("GOOGLE_PLAY_CREDENTIALS_JSON"),
		FacebookAppID:            os.Getenv("FACEBOOK_APP_ID"),
		FacebookAppSecret:        os.Getenv("FACEBOOK_APP_SECRET"),
		// EnvioEndpoint is the EmpowerTours Music NFT indexer (same one the
		// Farcaster miniapp uses). The URL rotates when the indexer is
		// redeployed, so keep it overridable via env.
		EnvioEndpoint:            getEnv("ENVIO_ENDPOINT", "https://indexer.hyperindex.xyz/179604b/v1/graphql"),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
