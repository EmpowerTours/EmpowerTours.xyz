ALTER TABLE users ADD COLUMN facebook_id TEXT;
CREATE INDEX IF NOT EXISTS idx_users_facebook ON users(facebook_id);
