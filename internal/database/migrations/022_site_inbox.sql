-- Inbox for the empowertours.xyz marketing site.
-- site_inquiries: "Start a project" briefs (plain text, sent by the visitor).
-- site_applications: careers applications. Only CIPHERTEXT is stored: the browser
-- encrypts with the hiring public key (RSA-OAEP-256 wrapping an AES-256-GCM key) and
-- the private key never reaches this server, so the API cannot read them.
CREATE TABLE IF NOT EXISTS site_inquiries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    project_type TEXT NOT NULL DEFAULT '',
    budget TEXT NOT NULL DEFAULT '',
    timeline TEXT NOT NULL DEFAULT '',
    repo TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'form',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS site_applications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    version INTEGER NOT NULL,
    kid TEXT NOT NULL,
    ek TEXT NOT NULL,
    iv TEXT NOT NULL,
    ct TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
