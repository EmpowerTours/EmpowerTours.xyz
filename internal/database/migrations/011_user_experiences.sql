-- Allow users to create their own experiences
ALTER TABLE experiences ADD COLUMN creator_id TEXT REFERENCES users(id);
ALTER TABLE experiences ADD COLUMN status TEXT NOT NULL DEFAULT 'active';  -- draft, pending_review, active, suspended
ALTER TABLE experiences ADD COLUMN duration_min INTEGER;
ALTER TABLE experiences ADD COLUMN meeting_point TEXT;
ALTER TABLE experiences ADD COLUMN what_to_bring TEXT;
ALTER TABLE experiences ADD COLUMN languages TEXT DEFAULT 'English';
ALTER TABLE experiences ADD COLUMN min_guests INTEGER DEFAULT 1;
ALTER TABLE experiences ADD COLUMN cover_photo_url TEXT;
ALTER TABLE experiences ADD COLUMN gallery_urls TEXT;  -- JSON array
ALTER TABLE experiences ADD COLUMN updated_at DATETIME DEFAULT CURRENT_TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_experiences_creator ON experiences(creator_id);
CREATE INDEX IF NOT EXISTS idx_experiences_status ON experiences(status);
