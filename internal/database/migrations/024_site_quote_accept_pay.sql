-- Client acceptance and SPEI/CoDi payment for site quotes.
-- status now runs: draft -> sent -> accepted -> paid (or draft -> discarded).
-- Token columns hold sha256(token) in hex; tokens are only ever emailed.
ALTER TABLE site_quotes ADD COLUMN client_token_hash TEXT NOT NULL DEFAULT '';   -- set on send; opens the accept page
ALTER TABLE site_quotes ADD COLUMN accepted_at DATETIME;
ALTER TABLE site_quotes ADD COLUMN accepted_name TEXT NOT NULL DEFAULT '';
ALTER TABLE site_quotes ADD COLUMN accepted_ip TEXT NOT NULL DEFAULT '';
ALTER TABLE site_quotes ADD COLUMN accepted_doc_sha256 TEXT NOT NULL DEFAULT ''; -- sha256 of id|currency|price|monthly|subject|body
ALTER TABLE site_quotes ADD COLUMN pay_reference TEXT NOT NULL DEFAULT '';       -- ET-Q0012, set on accept
ALTER TABLE site_quotes ADD COLUMN pay_token_hash TEXT NOT NULL DEFAULT '';      -- owner's verify link; cleared when paid
ALTER TABLE site_quotes ADD COLUMN paid_at DATETIME;
ALTER TABLE site_quotes ADD COLUMN paid_amount_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE site_quotes ADD COLUMN cep_fecha TEXT NOT NULL DEFAULT '';           -- fecha de operación, YYYY-MM-DD
ALTER TABLE site_quotes ADD COLUMN cep_clave_rastreo TEXT NOT NULL DEFAULT '';
ALTER TABLE site_quotes ADD COLUMN cep_emisor TEXT NOT NULL DEFAULT '';          -- Banxico participant code, e.g. 40012
ALTER TABLE site_quotes ADD COLUMN cep_receptor TEXT NOT NULL DEFAULT '';
ALTER TABLE site_quotes ADD COLUMN cep_xml TEXT NOT NULL DEFAULT '';             -- CEP XML as downloaded from Banxico
ALTER TABLE site_quotes ADD COLUMN cep_status TEXT NOT NULL DEFAULT '';          -- '' | verified | manual

-- One SPEI transfer can settle only one quote.
CREATE UNIQUE INDEX IF NOT EXISTS idx_site_quotes_cep_transfer
    ON site_quotes(cep_clave_rastreo, cep_fecha, cep_emisor) WHERE cep_clave_rastreo != '';
