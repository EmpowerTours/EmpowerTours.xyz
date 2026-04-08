-- Add email/password auth fields to users
ALTER TABLE users ADD COLUMN email_verified BOOLEAN DEFAULT 0;
ALTER TABLE users ADD COLUMN password_hash TEXT;
ALTER TABLE users ADD COLUMN auth_method TEXT DEFAULT 'wallet';  -- 'wallet', 'email', 'phone'
