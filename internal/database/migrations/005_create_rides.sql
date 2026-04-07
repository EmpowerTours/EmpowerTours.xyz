CREATE TABLE IF NOT EXISTS rides (
    id TEXT PRIMARY KEY,
    rider_id TEXT REFERENCES users(id),
    driver_id TEXT,
    pickup_lat REAL,
    pickup_lng REAL,
    pickup_address TEXT,
    dropoff_lat REAL,
    dropoff_lng REAL,
    dropoff_address TEXT,
    status TEXT DEFAULT 'requested',
    estimated_fare_mon REAL,
    actual_fare_mon REAL,
    payment_tx_hash TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);
