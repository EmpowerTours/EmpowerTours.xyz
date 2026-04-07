CREATE TABLE IF NOT EXISTS experiences (
    id TEXT PRIMARY KEY,
    slug TEXT UNIQUE,
    title TEXT NOT NULL,
    category TEXT NOT NULL,
    description TEXT,
    location_name TEXT,
    latitude REAL,
    longitude REAL,
    price_mon REAL,
    price_usd REAL,
    max_guests INTEGER,
    image_url TEXT,
    is_active BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
