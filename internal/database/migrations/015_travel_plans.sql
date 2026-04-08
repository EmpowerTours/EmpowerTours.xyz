CREATE TABLE IF NOT EXISTS travel_plans (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    destination TEXT NOT NULL,
    destination_lat REAL,
    destination_lng REAL,
    country TEXT,
    arrive_date DATE NOT NULL,
    depart_date DATE NOT NULL,
    notes TEXT,
    is_active BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_travel_plans_user ON travel_plans(user_id);
CREATE INDEX IF NOT EXISTS idx_travel_plans_dest ON travel_plans(destination);
CREATE INDEX IF NOT EXISTS idx_travel_plans_dates ON travel_plans(arrive_date, depart_date);
