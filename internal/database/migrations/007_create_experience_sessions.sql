-- An active experience session (a user doing an experience in real time)
CREATE TABLE IF NOT EXISTS experience_sessions (
    id TEXT PRIMARY KEY,
    booking_id TEXT NOT NULL REFERENCES bookings(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    guide_id TEXT REFERENCES users(id),
    experience_id TEXT NOT NULL REFERENCES experiences(id),
    status TEXT NOT NULL DEFAULT 'pending',  -- pending, active, paused, completed, cancelled
    started_at DATETIME,
    completed_at DATETIME,
    total_distance_meters REAL DEFAULT 0,
    avg_speed_kmh REAL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sessions_user ON experience_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_booking ON experience_sessions(booking_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON experience_sessions(status);
