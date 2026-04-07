-- GPS location breadcrumbs recorded during a session
CREATE TABLE IF NOT EXISTS location_history (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES experience_sessions(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    latitude REAL NOT NULL,
    longitude REAL NOT NULL,
    altitude REAL,
    accuracy REAL,
    speed REAL,
    heading REAL,
    recorded_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_location_session ON location_history(session_id);
CREATE INDEX IF NOT EXISTS idx_location_time ON location_history(session_id, recorded_at);
