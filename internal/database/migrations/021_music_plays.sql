-- Analytics-only play counts for the free music client.
-- Intentionally decoupled from any on-chain reward system: incrementing this
-- table never mints tokens and never pays anyone. It exists purely to show how
-- many times a track has been played. Economic payouts stay on the web/miniapp,
-- where identity and anti-abuse controls live.
CREATE TABLE IF NOT EXISTS music_plays (
    token_id TEXT PRIMARY KEY,
    play_count INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
