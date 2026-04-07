-- Photos taken at waypoint checkpoints during a session
CREATE TABLE IF NOT EXISTS checkpoint_photos (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES experience_sessions(id),
    waypoint_id TEXT NOT NULL REFERENCES waypoints(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    photo_url TEXT NOT NULL,
    latitude REAL NOT NULL,
    longitude REAL NOT NULL,
    caption TEXT,
    taken_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_photos_session ON checkpoint_photos(session_id);
CREATE INDEX IF NOT EXISTS idx_photos_waypoint ON checkpoint_photos(waypoint_id);
