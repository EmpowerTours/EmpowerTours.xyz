-- Instant quote drafts for briefs sent by the Claude project scoper.
-- Prices are computed by code from site_settings('pricing'); the AI only drafts the email text.
-- approve_token_hash is sha256(token) in hex; the token itself is only ever emailed to the owner.
CREATE TABLE IF NOT EXISTS site_quotes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    inquiry_id INTEGER NOT NULL REFERENCES site_inquiries(id),
    country TEXT NOT NULL DEFAULT '',     -- MX | US | CA | EU | GULF | OTHER
    lang TEXT NOT NULL DEFAULT 'en',
    region TEXT NOT NULL,
    currency TEXT NOT NULL,
    points INTEGER NOT NULL,
    credits INTEGER NOT NULL,
    price INTEGER NOT NULL,
    monthly INTEGER NOT NULL,
    manual INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'draft', -- draft | sent | discarded
    draft TEXT NOT NULL DEFAULT '{}',     -- AI draft JSON (summary, included, questions, flags, client_email)
    flags TEXT NOT NULL DEFAULT '[]',     -- JSON array: AI flags plus code flags ("price mismatch", "no AI draft")
    ai_at DATETIME,                       -- set when an AI draft is attempted; counted for the daily AI cap
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    approve_token_hash TEXT NOT NULL DEFAULT '',
    sent_subject TEXT NOT NULL DEFAULT '',
    sent_body TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    sent_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_site_quotes_ai_at ON site_quotes(ai_at);

CREATE TABLE IF NOT EXISTS site_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO site_settings (key, value) VALUES ('pricing',
'{"base_credits":12,"per_point":3,"min_credits":15,"regions":{"MX":{"credit":450,"hosting_low":299,"hosting_high":499},"INTL":{"credit":60,"hosting_low":29,"hosting_high":49},"ROW":{"credit":35,"hosting_low":19,"hosting_high":29}}}');
