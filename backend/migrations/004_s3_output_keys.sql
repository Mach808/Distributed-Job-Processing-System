-- Stage 5: persist the deterministic S3 key for processed job output.
ALTER TABLE jobs
ADD COLUMN IF NOT EXISTS output_key TEXT;
