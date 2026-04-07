-- Subscription tracking for IAP (Apple/Google)
CREATE TABLE IF NOT EXISTS subscriptions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    platform TEXT NOT NULL,  -- 'apple', 'google'
    product_id TEXT NOT NULL,  -- 'empowertours_annual'
    original_transaction_id TEXT UNIQUE,
    latest_receipt TEXT,
    status TEXT NOT NULL DEFAULT 'trialing',  -- trialing, active, expired, cancelled, grace_period
    trial_start DATETIME,
    trial_end DATETIME,
    current_period_start DATETIME,
    current_period_end DATETIME,
    auto_renew BOOLEAN DEFAULT 1,
    price_local INTEGER,  -- amount in local currency smallest unit (centavos)
    currency TEXT DEFAULT 'MXN',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_user ON subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_subscriptions_transaction ON subscriptions(original_transaction_id);

-- Add subscription fields to users
ALTER TABLE users ADD COLUMN subscription_status TEXT DEFAULT 'none';  -- none, trialing, active, expired
ALTER TABLE users ADD COLUMN trial_ends_at DATETIME;
ALTER TABLE users ADD COLUMN subscription_ends_at DATETIME;
