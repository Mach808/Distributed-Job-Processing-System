-- Search value for CSV processing jobs.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS query TEXT;
