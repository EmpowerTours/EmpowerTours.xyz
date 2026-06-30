-- 017_user_intent_and_presence.sql
-- Enhance profiles for empowered tours matching (artists, guides, barter/paid)
-- Add lightweight presence for "who's at / planning spots"

ALTER TABLE users ADD COLUMN empower_role TEXT DEFAULT 'traveler';  -- traveler | artist | guide | hybrid | local
ALTER TABLE users ADD COLUMN barter_ok BOOLEAN DEFAULT 0;
ALTER TABLE users ADD COLUMN paid_ok BOOLEAN DEFAULT 1;
ALTER TABLE users ADD COLUMN preferred_regions TEXT;  -- JSON array e.g. ["Mexico","Europe","SE Asia"]
ALTER TABLE users ADD COLUMN skills TEXT;             -- JSON array e.g. ["photography","guitar","spanish"]

-- Lightweight presence / planning (for bars, plazas, meet spots, experiences)
ALTER TABLE users ADD COLUMN current_spot TEXT;       -- e.g. "Rooftop Bar Roma", "Cenote Azul"
ALTER TABLE users ADD COLUMN current_spot_lat REAL;
ALTER TABLE users ADD COLUMN current_spot_lng REAL;
ALTER TABLE users ADD COLUMN planning_spot TEXT;
ALTER TABLE users ADD COLUMN planning_date TEXT;      -- YYYY-MM-DD or free "tonight"
ALTER TABLE users ADD COLUMN planning_spot_lat REAL;
ALTER TABLE users ADD COLUMN planning_spot_lng REAL;

-- Optional: simple spot check-ins / plans table for richer counts later
CREATE TABLE IF NOT EXISTS spot_attendance (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    spot_name TEXT NOT NULL,
    spot_city TEXT,
    lat REAL,
    lng REAL,
    status TEXT NOT NULL DEFAULT 'planning',  -- planning | here_now | left
    when_date TEXT,                           -- date or "tonight"
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_attendance_spot ON spot_attendance(spot_name, spot_city);
CREATE INDEX IF NOT EXISTS idx_attendance_user ON spot_attendance(user_id);
CREATE INDEX IF NOT EXISTS idx_attendance_status ON spot_attendance(status);
