# Distributed Job System

A Go service that accepts jobs over HTTP, stores them in PostgreSQL, and processes them with a pool of background workers. PostgreSQL is both the durable job store and the work queue. The web dashboard is embedded in the server binary and served from `/`.

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

The server also loads variables from a `.env` file in its working directory when present. `DATABASE_URL` is required. `PORT` is optional and defaults to `8080`.

## Run

From the repository root:

```sh
go run ./cmd/server
```

Open [http://localhost:8080](http://localhost:8080) to use the dashboard. The server starts three workers and a lease recovery loop alongside the HTTP API. Send `SIGINT` or `SIGTERM` for a graceful shutdown.

## HTTP API

### Create a job

`POST /jobs` accepts a JSON object with a `type` and integer operands `a` and `b`:

```sh
curl -i http://localhost:8080/jobs \
  -H 'Content-Type: application/json' \
  -d '{"type":"addition","a":12,"b":8}'
```

On success, the API returns `201 Created` and the job record, including its generated ID and initial `queued` status. Supported job types:

| Type | Behavior |
| --- | --- |
| `addition` | Adds `a` and `b`. |
| `subtraction` | Subtracts `b` from `a`. |
| `multiplication` | Multiplies `a` and `b`. |
| `division` | Integer-divides `a` by `b`; `b` must be nonzero. |
| `fibonacci` | Returns Fibonacci number `a`; `a` must be from 0 through 46. |
| `prime_count` | Counts primes up to `a`; `a` must be from 2 through 10,000,000. |
| `retry_demo` | Fails for the first `a` attempts, then returns `b`; `a` must be from 0 through 2. |

Unknown fields, malformed JSON, unsupported job types, and invalid inputs return `400 Bad Request`.

### Get a job

Use the ID returned by job creation:

```sh
curl http://localhost:8080/jobs/job-<id>
```

The response includes the job's status and result, retry count, current attempt, and worker/lease details when applicable. A missing ID returns `404 Not Found`.

Jobs move through `queued`, `processing`, and either `completed` or `failed`. Workers claim jobs in PostgreSQL, renew time-limited leases while processing, and reject stale attempts from completing a job. Failed attempts and expired leases are retried up to the configured limit (three attempts); the recovery loop runs every two seconds.

## Project layout

- `cmd/server/` — HTTP server, PostgreSQL job store, workers, recovery loop, and dashboard assets.
- `channel_ex.go` — standalone channel example.

