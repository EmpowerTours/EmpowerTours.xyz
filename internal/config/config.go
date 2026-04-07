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
		StripeSecretKey:         os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:    os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
