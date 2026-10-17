-- Local file inputs for image, log, and CSV jobs.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS input_path TEXT;
