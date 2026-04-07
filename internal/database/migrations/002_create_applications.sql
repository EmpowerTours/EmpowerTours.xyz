CREATE TABLE IF NOT EXISTS applications (
    id TEXT PRIMARY KEY,
    user_id TEXT REFERENCES users(id),
    role TEXT NOT NULL DEFAULT 'customer',  -- 'customer', 'worker', 'creator'
    full_name TEXT NOT NULL,
    email TEXT NOT NULL,
    phone TEXT,
    location TEXT,                          -- city/town in Guerrero
    reason TEXT NOT NULL,
    referral_code TEXT,

    -- consent (required for all)
    content_consent INTEGER NOT NULL DEFAULT 0,  -- agreed to be recorded/used for brand

    -- worker-specific
    skills TEXT,              -- JSON array: ["driving","farming","landscaping","masonry","guiding","animal_care"]
    vehicle_type TEXT,        -- if driver: car, van, suv, atv, etc.
    availability TEXT,        -- 'fulltime', 'parttime', 'weekends'

    -- worker documents (required before interview)
    ine_doc_url TEXT,                -- INE (Mexican ID) upload
    recommendation_letter_url TEXT,  -- letter of recommendation upload
    current_job_situation TEXT,      -- description of current employment
    documents_complete INTEGER DEFAULT 0,  -- all required docs submitted

    -- interview scheduling (admin sets after doc review)
    interview_status TEXT DEFAULT 'none',  -- 'none', 'docs_pending', 'docs_submitted', 'scheduled', 'completed', 'no_show'
    interview_date DATETIME,
    interview_location TEXT,
    interview_notes TEXT,            -- admin notes from interview

    -- customer-specific
    interests TEXT,           -- JSON array: ["ranch","climbing","dining","transport"]
    group_size INTEGER,

    status TEXT DEFAULT 'pending',  -- 'pending', 'docs_required', 'interview_scheduled', 'approved', 'rejected'
    reviewed_by TEXT,
    reviewed_at DATETIME,
    tx_hash TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
