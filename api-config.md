# Distributed Job System API

These examples use WSL/Bash and assume the server is running at
`http://localhost:8080`.

```bash
BASE_URL="http://localhost:8080"
```

Do not use PowerShell backticks in WSL. Bash uses `\` for multiline commands.

## Authentication

The API uses one of these mechanisms:

- Admin endpoints: `X-Admin-Key`
- Job endpoints: `X-API-Key` or the `job_session` cookie returned by login

The API key is returned only when an API client is created. Keep it secret.

## Create an API client

The server must have been started with `ADMIN_API_KEY` configured.

```bash
curl -i -X POST "$BASE_URL/admin/clients" \
  -H "X-Admin-Key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"client_id":"client-1"}'
```

Expected response: `201 Created`

```json
{
  "client_id": "client-1",
  "api_key": "ck_..."
}
```

Save the returned key:

```bash
export API_KEY="ck_your_key_here"
```

## Create a user

Users are associated with an existing API client. Passwords must be at least
12 characters.

```bash
curl -i -X POST "$BASE_URL/admin/users" \
  -H "X-Admin-Key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "username":"alice",
    "password":"a-long-test-password",
    "client_id":"client-1"
  }'
```

Expected response: `201 Created`.

## Login

Login returns a `job_session` cookie. `-c cookies.txt` saves it for later
requests.

```bash
curl -i -c cookies.txt -X POST "$BASE_URL/login" \
  -H "Content-Type: application/json" \
  -d '{
    "username":"alice",
    "password":"a-long-test-password"
  }'
```

Expected response: `200 OK` with the client ID in the JSON body.

## Create a job with an API key

This is why a request without authentication returns `401 authentication
required`.

```bash
curl -i -X POST "$BASE_URL/jobs" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"type":"addition","a":10,"b":5}'
```

Expected response: `201 Created`. The response contains a generated job ID,
for example `job-123456789`.

The response also includes rate-limit headers:

- `X-RateLimit-Limit`
- `X-RateLimit-Remaining`
- `Retry-After` on `429 Too Many Requests`

## Create a job with the login cookie

```bash
curl -i -b cookies.txt -X POST "$BASE_URL/jobs" \
  -H "Content-Type: application/json" \
  -d '{"type":"multiplication","a":6,"b":7}'
```

## Supported job requests

All requests below use the same authentication header:

```bash
-H "X-API-Key: $API_KEY"
```

### Numeric jobs

```bash
# subtraction
curl -i -X POST "$BASE_URL/jobs" -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" -d '{"type":"subtraction","a":10,"b":3}'

# division; b cannot be zero
curl -i -X POST "$BASE_URL/jobs" -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" -d '{"type":"division","a":20,"b":4}'

# fibonacci; a must be 0 through 46
curl -i -X POST "$BASE_URL/jobs" -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" -d '{"type":"fibonacci","a":10}'

# prime_count; a must be 2 through 10000000
curl -i -X POST "$BASE_URL/jobs" -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" -d '{"type":"prime_count","a":1000}'

# retry_demo; a must be 0, 1, or 2, and b is the result
curl -i -X POST "$BASE_URL/jobs" -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" -d '{"type":"retry_demo","a":1,"b":42}'
```

### Local-file jobs

`input_path` must be a relative path inside the configured `JOB_DATA_DIR`,
which defaults to `backend/data/jobs`.

```bash
# Existing example files include sample.png, sample.log, and sample.csv.
curl -i -X POST "$BASE_URL/jobs" -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" -d '{"type":"image_processing","input_path":"sample.png"}'

curl -i -X POST "$BASE_URL/jobs" -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" -d '{"type":"log_analysis","input_path":"sample.log"}'

curl -i -X POST "$BASE_URL/jobs" -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" -d '{"type":"csv_processing","input_path":"sample.csv","query":"Alice"}'
```

## Get a job

Replace `JOB_ID` with the ID returned when the job was created.

```bash
export JOB_ID="job-123456789"

curl -i "$BASE_URL/jobs/$JOB_ID" \
  -H "X-API-Key: $API_KEY"
```

Expected responses are `200 OK` or `404 Not Found` if the ID does not exist.

## List recent jobs

```bash
curl -i "$BASE_URL/jobs" \
  -H "X-API-Key: $API_KEY"
```

The endpoint returns up to the 100 most recent jobs.

## Logout

This invalidates the login session cookie. It does not revoke an API key.

```bash
curl -i -b cookies.txt -X POST "$BASE_URL/logout"
```

Expected response: `204 No Content`.

## Rate-limit test

The repository includes a concurrent WSL/Bash test script:

```bash
cd /mnt/d/code/distributed-job-system/backend
bash ./test_rate_limit.sh "$API_KEY" 40 http://localhost:8080
```

To test multiple server instances, run the command in separate terminals with
ports `8080`, `8081`, and `8082`. They must share the same PostgreSQL database.

## Common responses

| Status | Meaning |
|---|---|
| `201` | Job or API client created |
| `200` | Request succeeded |
| `204` | Logout succeeded |
| `400` | Invalid JSON or job input |
| `401` | Missing or invalid authentication |
| `404` | Resource not found, or admin API disabled |
| `409` | Duplicate API client or user |
| `429` | Rate limit exceeded; check `Retry-After` |
| `500` | Server or database error |
