-- User profiles/bios for the Chill feature
ALTER TABLE users ADD COLUMN bio TEXT;
ALTER TABLE users ADD COLUMN profile_photo_url TEXT;
ALTER TABLE users ADD COLUMN interests TEXT;  -- JSON array: ["hiking","food","nightlife"]
ALTER TABLE users ADD COLUMN current_lat REAL;
ALTER TABLE users ADD COLUMN current_lng REAL;
ALTER TABLE users ADD COLUMN current_city TEXT;
ALTER TABLE users ADD COLUMN age INTEGER;
ALTER TABLE users ADD COLUMN is_discoverable BOOLEAN DEFAULT 1;

-- Chill requests (swipe-style: one user sends, other accepts/declines)
CREATE TABLE IF NOT EXISTS chill_requests (
    id TEXT PRIMARY KEY,
    from_user_id TEXT NOT NULL REFERENCES users(id),
    to_user_id TEXT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL DEFAULT 'pending',  -- pending, accepted, declined, expired
    message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(from_user_id, to_user_id)
);

CREATE INDEX IF NOT EXISTS idx_chill_from ON chill_requests(from_user_id);
CREATE INDEX IF NOT EXISTS idx_chill_to ON chill_requests(to_user_id);
CREATE INDEX IF NOT EXISTS idx_chill_status ON chill_requests(status);

-- Chill matches (created when both users accept)
CREATE TABLE IF NOT EXISTS chill_matches (
    id TEXT PRIMARY KEY,
    user1_id TEXT NOT NULL REFERENCES users(id),
    user2_id TEXT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL DEFAULT 'matched',  -- matched, scheduled, completed, cancelled
    -- Midpoint meeting location
    meet_lat REAL,
    meet_lng REAL,
    meet_location_name TEXT,
    meet_date DATETIME,
    meet_experience_id TEXT REFERENCES experiences(id),
    -- Chat
    last_message TEXT,
    last_message_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_match_user1 ON chill_matches(user1_id);
CREATE INDEX IF NOT EXISTS idx_match_user2 ON chill_matches(user2_id);
