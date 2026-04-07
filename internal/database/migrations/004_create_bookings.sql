CREATE TABLE IF NOT EXISTS bookings (
    id TEXT PRIMARY KEY,
    user_id TEXT REFERENCES users(id),
    experience_id TEXT REFERENCES experiences(id),
    booking_date TEXT,
    guest_count INTEGER,
    status TEXT DEFAULT 'pending',
    payment_tx_hash TEXT,
    payment_status TEXT DEFAULT 'unpaid',
    total_mon REAL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
