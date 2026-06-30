-- Presence for real nearby/world-map counts.
ALTER TABLE users ADD COLUMN presence_updated_at DATETIME;
ALTER TABLE users ADD COLUMN presence_expires_at DATETIME;
ALTER TABLE users ADD COLUMN precise_location_ok BOOLEAN DEFAULT 0;

-- Stripe Connect payout onboarding for hosts/workers.
ALTER TABLE users ADD COLUMN stripe_account_id TEXT;
ALTER TABLE users ADD COLUMN stripe_charges_enabled BOOLEAN DEFAULT 0;
ALTER TABLE users ADD COLUMN stripe_payouts_enabled BOOLEAN DEFAULT 0;
ALTER TABLE users ADD COLUMN stripe_details_submitted BOOLEAN DEFAULT 0;

-- Booking payout state for real-world marketplace transactions.
ALTER TABLE bookings ADD COLUMN payout_status TEXT DEFAULT 'not_ready';
ALTER TABLE bookings ADD COLUMN stripe_transfer_id TEXT;
ALTER TABLE bookings ADD COLUMN platform_fee_cents INTEGER DEFAULT 0;

CREATE TABLE IF NOT EXISTS payouts (
    id TEXT PRIMARY KEY,
    booking_id TEXT NOT NULL REFERENCES bookings(id),
    provider_user_id TEXT NOT NULL REFERENCES users(id),
    payment_id TEXT REFERENCES payments(id),
    stripe_transfer_id TEXT UNIQUE,
    amount_cents INTEGER NOT NULL,
    platform_fee_cents INTEGER NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'usd',
    status TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_payouts_booking ON payouts(booking_id);
CREATE INDEX IF NOT EXISTS idx_payouts_provider ON payouts(provider_user_id);
