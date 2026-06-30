-- 018_lifetime_membership_and_free_explore.sql
-- One-time lifetime membership ($100) + daily free explore quota

-- Lifetime membership fields (reuses existing membership_tier)
ALTER TABLE users ADD COLUMN lifetime_paid_at DATETIME;
ALTER TABLE users ADD COLUMN membership_amount_cents INTEGER DEFAULT 0;

-- Daily free explore usage (1 free use per day to view content in Explore)
ALTER TABLE users ADD COLUMN free_explore_last_date DATE;
ALTER TABLE users ADD COLUMN free_explore_uses_today INTEGER DEFAULT 0;

-- Optional: record lifetime purchases separately
CREATE TABLE IF NOT EXISTS lifetime_memberships (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    stripe_payment_intent_id TEXT UNIQUE,
    amount_cents INTEGER NOT NULL DEFAULT 10000, -- $100.00
    currency TEXT DEFAULT 'usd',
    status TEXT DEFAULT 'pending', -- pending, succeeded, failed
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_lifetime_user ON lifetime_memberships(user_id);
CREATE INDEX IF NOT EXISTS idx_lifetime_stripe ON lifetime_memberships(stripe_payment_intent_id);

-- Seed or ensure membership_tier can be 'lifetime'
-- (existing membership_tier field is reused)