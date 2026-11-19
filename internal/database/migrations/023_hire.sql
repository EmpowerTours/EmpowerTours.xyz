-- Hire portal (empowertours.xyz/hire): clients connect GitHub repos, approve
-- fixed-price tasks, merge our pull requests, then pay each merged task.
-- One task = one pull request = one payment. There is no project total.

CREATE TABLE IF NOT EXISTS hire_clients (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    github_id INTEGER NOT NULL UNIQUE,
    login TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Only the SHA-256 of the session token is stored.
CREATE TABLE IF NOT EXISTS hire_sessions (
    token_hash TEXT PRIMARY KEY,
    client_id INTEGER NOT NULL REFERENCES hire_clients(id),
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Fiscal data for the CFDI 4.0 issued per paid task.
CREATE TABLE IF NOT EXISTS hire_billing (
    client_id INTEGER PRIMARY KEY REFERENCES hire_clients(id),
    rfc TEXT NOT NULL,
    legal_name TEXT NOT NULL,
    regimen TEXT NOT NULL,
    cp TEXT NOT NULL,
    uso_cfdi TEXT NOT NULL,
    email TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- GitHub App installations. sender_github_id is the GitHub user who installed it;
-- the installation belongs to the client with that github_id.
CREATE TABLE IF NOT EXISTS hire_installations (
    installation_id INTEGER PRIMARY KEY,
    sender_github_id INTEGER NOT NULL,
    account_login TEXT NOT NULL,
    removed_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS hire_repos (
    repo_id INTEGER PRIMARY KEY,
    installation_id INTEGER NOT NULL REFERENCES hire_installations(installation_id),
    full_name TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_hire_repos_inst ON hire_repos(installation_id);

CREATE TABLE IF NOT EXISTS hire_projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id INTEGER NOT NULL REFERENCES hire_clients(id),
    repo_full_name TEXT NOT NULL,
    title TEXT NOT NULL,
    brief TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_hire_projects_client ON hire_projects(client_id);

-- stage: bug -> quoted -> approved -> building -> review -> merged -> paid (or cancelled).
-- price_mxn is whole pesos, always > 0 once quoted; NULL only while a bug report waits for its price.
-- Nothing is free: every bug is a new priced task.
CREATE TABLE IF NOT EXISTS hire_tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES hire_projects(id),
    kind TEXT NOT NULL DEFAULT 'feature',
    parent_task_id INTEGER REFERENCES hire_tasks(id),
    title TEXT NOT NULL,
    what TEXT NOT NULL DEFAULT '',
    acceptance TEXT NOT NULL DEFAULT '[]',
    price_mxn INTEGER,
    stage TEXT NOT NULL,
    pr_number INTEGER,
    pr_url TEXT NOT NULL DEFAULT '',
    approved_at DATETIME,
    merged_at DATETIME,
    merge_sha TEXT NOT NULL DEFAULT '',
    paid_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_hire_tasks_project ON hire_tasks(project_id);

-- One confirmed payment per task. MON intents lock a rate and an exact amount
-- (unique among open intents) for 30 minutes; tx_hash can never be reused.
CREATE TABLE IF NOT EXISTS hire_payments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER NOT NULL REFERENCES hire_tasks(id),
    method TEXT NOT NULL,
    amount_mxn INTEGER NOT NULL,
    amount_wei TEXT NOT NULL DEFAULT '',
    rate_mxn_per_mon TEXT NOT NULL DEFAULT '',
    pay_to TEXT NOT NULL DEFAULT '',
    reference TEXT,
    tx_hash TEXT UNIQUE,
    status TEXT NOT NULL,
    expires_at DATETIME,
    confirmed_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_hire_payments_one_confirmed ON hire_payments(task_id) WHERE status = 'confirmed';
CREATE INDEX IF NOT EXISTS idx_hire_payments_task ON hire_payments(task_id);

CREATE TABLE IF NOT EXISTS hire_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES hire_projects(id),
    task_id INTEGER,
    kind TEXT NOT NULL,
    text TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_hire_events_project ON hire_events(project_id);
