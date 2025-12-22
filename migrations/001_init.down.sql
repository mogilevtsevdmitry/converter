DROP TRIGGER IF EXISTS update_conversion_jobs_updated_at ON conversion_jobs;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP TABLE IF EXISTS conversion_artifacts;
DROP TABLE IF EXISTS conversion_errors;
DROP TABLE IF EXISTS conversion_jobs;
