CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    wallet_address TEXT UNIQUE NOT NULL,
    display_name TEXT,
    email TEXT,
    phone TEXT,
    role TEXT DEFAULT 'customer',  -- 'customer', 'worker'
    membership_tier TEXT,
    membership_token_id INTEGER,
    nonce TEXT,
    is_admin BOOLEAN DEFAULT 0,
    is_driver BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
