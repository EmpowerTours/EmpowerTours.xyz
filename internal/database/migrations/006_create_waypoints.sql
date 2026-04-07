-- Waypoints define the GPS route/checkpoints for an experience
CREATE TABLE IF NOT EXISTS waypoints (
    id TEXT PRIMARY KEY,
    experience_id TEXT NOT NULL REFERENCES experiences(id),
    name TEXT NOT NULL,
    description TEXT,
    latitude REAL NOT NULL,
    longitude REAL NOT NULL,
    order_index INTEGER NOT NULL DEFAULT 0,
    radius_meters REAL NOT NULL DEFAULT 50,
    is_photo_checkpoint BOOLEAN NOT NULL DEFAULT 0,
    photo_prompt TEXT,
    estimated_duration_min INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_waypoints_experience ON waypoints(experience_id);
