# Distributed Job System

A Go service that accepts jobs over HTTP, stores them in PostgreSQL, and processes them with a pool of background workers. PostgreSQL is both the durable job store and the work queue. The web dashboard lives in the root `website/` directory and is served from `/` by the Go server.

## Requirements

- Go 1.27.1 or newer
- PostgreSQL

## Database setup

Create a database, then create the table used by the service:

```sql
CREATE TABLE jobs (
    id          TEXT PRIMARY KEY,
    type        TEXT NOT NULL,
    a           INTEGER NOT NULL,
    b           INTEGER NOT NULL,
    input_path  TEXT,
    status      TEXT NOT NULL CHECK (status IN ('queued', 'processing', 'completed', 'failed')),
    result      INTEGER,
    retries     INTEGER NOT NULL DEFAULT 0,
    worker_id   TEXT,
    attempt     INTEGER NOT NULL DEFAULT 0,
    lease_until TIMESTAMPTZ
);

CREATE INDEX jobs_queued_id_idx ON jobs (id) WHERE status = 'queued';
CREATE INDEX jobs_expired_lease_idx ON jobs (lease_until) WHERE status = 'processing';
```

Set `DATABASE_URL` to a PostgreSQL connection string. For example:

```sh
export DATABASE_URL='postgres://postgres:password@localhost:5432/jobs?sslmode=disable'
```

The server loads variables from `backend/.env` when started with `backend/` as its working directory. `DATABASE_URL` is required. `PORT` is optional and defaults to `8080`. The dashboard files live in the root `website/` directory; set `WEBSITE_DIR` if the server cannot resolve that directory from its working directory.

## Run

From the repository root:

```sh
cd backend
go run ./cmd/server
```

Open [http://localhost:8080](http://localhost:8080) to use the dashboard. The server starts three workers and a lease recovery loop alongside the HTTP API. Send `SIGINT` or `SIGTERM` for a graceful shutdown.

## HTTP API

### Create a job

`POST /jobs` accepts a JSON object with a `type`. Numeric jobs use integer
operands `a` and `b`; S3-backed file jobs use an existing `input_key` below
the bucket's `input/` prefix, and CSV searches also use `query`:

```sh
curl -i http://localhost:8080/jobs \
  -H 'Content-Type: application/json' \
  -d '{"type":"addition","a":12,"b":8}'
```

On success, the API returns `201 Created` and the job record, including its generated ID and initial `queued` status. Supported job types:

| Type | Description | Inputs | Result |
| --- | --- | --- | --- |
| `addition` | Adds two numbers. | `a`, `b` | `a + b` |
| `subtraction` | Subtracts the second number from the first. | `a`, `b` | `a - b` |
| `multiplication` | Multiplies two numbers. | `a`, `b` | `a × b` |
| `division` | Performs integer division; `b` must be nonzero. | `a`, `b` | `a / b` |
| `fibonacci` | Calculates a Fibonacci number; `a` must be 0–46. | `a` | Fibonacci value |
| `prime_count` | Counts primes up to `a`; `a` must be 2–10,000,000. | `a` | Prime count |
| `retry_demo` | Fails for the first `a` attempts, then succeeds. | `a` retries, `b` result | `b` |
| `image_processing` | Validates an existing S3 image object. | `input_key` | Not processed from S3 until a later stage |
| `log_analysis` | References an existing S3 log object. | `input_key` | Not processed from S3 until a later stage |
| `csv_processing` | References an existing S3 CSV object. | `input_key`, `query` | Not processed from S3 until a later stage |

Apply `backend/migrations/001_local_job_inputs.sql`,
`backend/migrations/002_csv_query.sql`, and
`backend/migrations/003_s3_input_keys.sql`. File-job keys must be under
`input/`; the API verifies that the referenced S3 object exists before queuing.

Unknown fields, malformed JSON, unsupported job types, and invalid inputs return `400 Bad Request`.

### Get a job

Use the ID returned by job creation:

```sh
curl http://localhost:8080/jobs/job-<id>
```

The response includes the job's status and result, retry count, current attempt, and worker/lease details when applicable. Completed S3-backed file jobs also include their private `output_key`. A missing ID returns `404 Not Found`.

For a completed file-processing job, `GET /jobs/{id}/download` returns a
short-lived presigned S3 URL. The URL lifetime defaults to 15 minutes and can
be configured with `S3_PRESIGNED_URL_EXPIRY_SECONDS` (up to 7 days). The
endpoint signs only the stored `output_key` under `output/`; it never accepts
an arbitrary S3 key from the client.

Jobs move through `queued`, `processing`, and either `completed` or `failed`. Workers claim jobs in PostgreSQL, renew time-limited leases while processing, and reject stale attempts from completing a job. Failed attempts and expired leases are retried up to the configured limit (three attempts); the recovery loop runs every two seconds.

## Project layout

- `backend/` — Go module, HTTP server, PostgreSQL job store, workers, recovery loop, and standalone channel example.
- `website/` — dashboard HTML, CSS, and vanilla JavaScript served by the Go server from disk.
- `mobile/` — reserved for the planned mobile frontend.
- `plan.md` — AWS deployment plan and proposed file-processing job types.

Run backend tests from the backend directory with `go test ./...`.

