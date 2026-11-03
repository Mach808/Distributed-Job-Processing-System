-- Stage 3: reference existing objects in the S3 input/ prefix.
ALTER TABLE jobs
ADD COLUMN IF NOT EXISTS input_key TEXT;
