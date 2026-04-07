-- Stripe payment records
CREATE TABLE IF NOT EXISTS payments (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    booking_id TEXT REFERENCES bookings(id),
    stripe_payment_intent_id TEXT UNIQUE,
    stripe_customer_id TEXT,
    amount_cents INTEGER NOT NULL,
    currency TEXT NOT NULL DEFAULT 'usd',
    status TEXT NOT NULL DEFAULT 'pending',  -- pending, succeeded, failed, refunded
    description TEXT,
    receipt_url TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_payments_user ON payments(user_id);
CREATE INDEX IF NOT EXISTS idx_payments_booking ON payments(booking_id);
CREATE INDEX IF NOT EXISTS idx_payments_stripe ON payments(stripe_payment_intent_id);

-- Add Stripe fields to bookings
ALTER TABLE bookings ADD COLUMN stripe_payment_id TEXT REFERENCES payments(id);
ALTER TABLE bookings ADD COLUMN total_usd_cents INTEGER;

-- Add Stripe customer to users
ALTER TABLE users ADD COLUMN stripe_customer_id TEXT;
