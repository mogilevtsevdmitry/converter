-- Conversion jobs table
CREATE TABLE IF NOT EXISTS conversion_jobs (
    id UUID PRIMARY KEY,
    video_id UUID,
    source_bucket TEXT NOT NULL,
    source_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'QUEUED',
    current_stage TEXT,
    stage_progress INT NOT NULL DEFAULT 0,
    overall_progress INT NOT NULL DEFAULT 0,
    profile JSONB NOT NULL,
    idempotency_key TEXT,
    workflow_id TEXT,
    priority INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    attempt INT NOT NULL DEFAULT 0,
    last_error_id UUID,
    lock_version INT NOT NULL DEFAULT 0
);

-- Unique index for idempotency key
CREATE UNIQUE INDEX IF NOT EXISTS idx_conversion_jobs_idempotency_key
    ON conversion_jobs (idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- Index for status queries
CREATE INDEX IF NOT EXISTS idx_conversion_jobs_status_updated
    ON conversion_jobs (status, updated_at);

-- Index for video_id lookups
CREATE INDEX IF NOT EXISTS idx_conversion_jobs_video_id
    ON conversion_jobs (video_id)
    WHERE video_id IS NOT NULL;

-- Index for current stage
CREATE INDEX IF NOT EXISTS idx_conversion_jobs_stage
    ON conversion_jobs (current_stage)
    WHERE current_stage IS NOT NULL;

-- Conversion errors table
CREATE TABLE IF NOT EXISTS conversion_errors (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES conversion_jobs(id) ON DELETE CASCADE,
    stage TEXT NOT NULL,
    class TEXT NOT NULL,
    code TEXT NOT NULL,
    message TEXT NOT NULL,
    details JSONB,
    attempt INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for job errors
CREATE INDEX IF NOT EXISTS idx_conversion_errors_job_created
    ON conversion_errors (job_id, created_at DESC);

-- Conversion artifacts table
CREATE TABLE IF NOT EXISTS conversion_artifacts (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES conversion_jobs(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    bucket TEXT NOT NULL,
    key TEXT NOT NULL,
    size_bytes BIGINT,
    checksum TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for job artifacts
CREATE INDEX IF NOT EXISTS idx_conversion_artifacts_job_type
    ON conversion_artifacts (job_id, type);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to auto-update updated_at
CREATE TRIGGER update_conversion_jobs_updated_at
    BEFORE UPDATE ON conversion_jobs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
