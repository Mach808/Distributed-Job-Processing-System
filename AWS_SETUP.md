# AWS Setup Guide for the Distributed Job System

This guide sets up the application on an EC2 instance with a private S3 bucket. The application uses the EC2 IAM role to access S3; no long-lived AWS access keys are required on the server.

## Architecture

```text
EC2 instance
  └── IAM instance role: DJS-EC2-S3-Access
        └── S3 permissions: list input files, read input/output objects, write output objects

S3 bucket: djs-s3-files-bucket
  ├── input/       uploaded source files
  └── output/      processed files
```

## 1. Create the S3 bucket

1. Open **AWS Console → S3 → Create bucket**.
2. Use a globally unique bucket name. This project uses:

   ```text
   djs-s3-files-bucket
   ```

3. Select the same AWS Region that will be used by the application. This project uses:

   ```text
   Asia Pacific (Sydney) / ap-southeast-2
   ```

4. Keep **Block all public access** enabled.
5. Keep **Object Ownership: Bucket owner enforced** enabled.
6. Create the bucket.
7. Open the bucket and create these folders if needed:

   ```text
   input/
   output/
   ```

Never make this bucket publicly readable or writable. The application returns presigned URLs for private output files.

## 2. Create the EC2 IAM role

1. Open **AWS Console → IAM → Roles → Create role**.
2. Select **AWS service** as the trusted entity.
3. Select **EC2** as the use case.
4. Continue without attaching broad AWS-managed permissions.
5. Name the role:

   ```text
   DJS-EC2-S3-Access
   ```

6. Create the role.

The EC2 trust relationship should allow the EC2 service to assume the role. Do not use an IAM user access key in the application.

## 3. Add the S3 permissions to the role

1. Open **IAM → Roles → DJS-EC2-S3-Access**.
2. Select **Add permissions → Create inline policy**.
3. Select the **JSON** editor.
4. Paste this policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ListInputFiles",
      "Effect": "Allow",
      "Action": "s3:ListBucket",
      "Resource": "arn:aws:s3:::djs-s3-files-bucket",
      "Condition": {
        "StringLike": {
          "s3:prefix": [
            "input",
            "input/*"
          ]
        }
      }
    },
    {
      "Sid": "ReadInputsWriteOutputs",
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject"
      ],
      "Resource": [
        "arn:aws:s3:::djs-s3-files-bucket/input/*",
        "arn:aws:s3:::djs-s3-files-bucket/output/*"
      ]
    }
  ]
}
```

5. Name the policy:

   ```text
   DJS-S3-Application-Access
   ```

6. Create the policy.

The application does not need `s3:DeleteObject`. Add it only if a future feature explicitly deletes files.

## 4. Keep the bucket policy private

Open **S3 → djs-s3-files-bucket → Permissions**.

Delete any bucket policy containing:

```json
"Principal": "*"
```

If a bucket policy is required, it must name the specific backend role instead of `*`. For this setup, an empty bucket policy is acceptable because the role's identity policy grants access.

Enable all four **Block public access** settings and save them:

- Block public access to buckets and objects granted through new access control lists (ACLs)
- Block public access to buckets and objects granted through any access control lists (ACLs)
- Block public access to buckets and objects granted through new public bucket or access point policies
- Block public and cross-account access to buckets and objects through any public bucket or access point policies

## 5. Create the EC2 instance

1. Open **AWS Console → EC2 → Launch instance**.
2. Choose an Ubuntu LTS or Amazon Linux image.
3. Select an instance size appropriate for the workload, such as `t3.micro` for a small demonstration.
4. Create or select an SSH key pair and store the private key securely.
5. Configure the security group:

   | Port | Source | Purpose |
   | --- | --- | --- |
   | 22 | Your IP address only | SSH administration |
   | 8080 | Your IP or trusted load balancer only | Application API, if exposed directly |

   Do not open SSH to `0.0.0.0/0`. If the application is behind a reverse proxy or load balancer, expose only the required public ports there.

6. In **Advanced details → IAM instance profile**, select:

   ```text
   DJS-EC2-S3-Access
   ```

7. Launch the instance.

If the role is not available in the instance profile list, create an instance profile or attach the role after launch through **Actions → Security → Modify IAM role**.

## 6. Install the software required on EC2

After connecting to the instance over SSH, install the tools needed to download,
build, verify, and run the backend. The commands below assume a 64-bit Linux
instance.

### Ubuntu

```sh
sudo apt update
sudo apt install -y ca-certificates curl git unzip postgresql-client build-essential
```

Install the Go version required by this repository (`go 1.27.1` in
`backend/go.mod`):

```sh
curl -LO https://go.dev/dl/go1.27.1.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.27.1.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
export PATH=$PATH:/usr/local/go/bin
go version
```

Install AWS CLI version 2:

```sh
curl -fsSL https://awscli.amazonaws.com/v2/install.sh | sudo bash
aws --version
```

### Amazon Linux

```sh
sudo dnf install -y ca-certificates curl git unzip postgresql15
```

On Amazon Linux 2, use `yum` instead:

```sh
sudo yum install -y ca-certificates curl git unzip postgresql
```

Install Go and AWS CLI version 2 using the same commands shown for Ubuntu.
If the instance uses ARM64, download the matching `linux-arm64` Go archive
instead of `linux-amd64`.

Confirm that all required commands are available:

```sh
go version
aws --version
git --version
curl --version
psql --version
```

The backend requires a reachable PostgreSQL database. `postgresql-client` or
`postgresql15` only provides the `psql` command for connecting to a database;
it does not create a database server. Use an existing database service such as
RDS, or install and configure PostgreSQL on the EC2 instance separately.

The current backend does not make Redis calls, so Redis is not required to
build or start the server even though `REDIS_URL` may appear in older example
configuration.

## 7. Verify the role from EC2

SSH into the instance and verify that the instance received the role credentials:

```sh
aws sts get-caller-identity
```

The result should identify the `DJS-EC2-S3-Access` role. Then test the permitted S3 operations:

```sh
aws s3 ls s3://djs-s3-files-bucket/input/ --region ap-southeast-2
echo "test" > /tmp/s3-test.txt
aws s3 cp /tmp/s3-test.txt s3://djs-s3-files-bucket/input/s3-test.txt --region ap-southeast-2
aws s3 cp s3://djs-s3-files-bucket/input/s3-test.txt /tmp/s3-test-download.txt --region ap-southeast-2
```

The AWS CLI must be installed for these checks. These commands use the EC2 role automatically; do not provide access keys on the command line.

## 8. Configure the application

Run the server from the `backend` directory so it loads `backend/.env`:

```env
S3_BUCKET=djs-s3-files-bucket
AWS_REGION=ap-southeast-2
PORT=8080
DATABASE_URL=your-postgresql-connection-string
REDIS_URL=redis://localhost:6379
```

`S3_PRESIGNED_URL_EXPIRY_SECONDS` is optional and defaults to 900 seconds (15 minutes).

On EC2, do not add these variables unless you have a specific reason:

```env
AWS_ACCESS_KEY_ID=...
AWS_SECRET_ACCESS_KEY=...
```

The SDK should obtain temporary credentials from the attached role.

Do not commit `.env` files or credentials to Git. Add the environment file to `.gitignore` and use a secret manager or protected server configuration for production secrets.

## 9. Build and run the backend

From the repository's `backend` directory:

```sh
go mod download
go build -o djs-server ./cmd/server
./djs-server
```

The process should start without:

```text
S3 configuration failed: S3_BUCKET is not set
S3 configuration failed: AWS_REGION is not set
```

For production, run the binary with `systemd`, Docker, or another process supervisor so it restarts after a failure or reboot.

## 10. Verify the application

Check the S3 listing endpoint:

```sh
curl http://localhost:8080/s3/files
```

Create a file-processing job using an object below `input/`:

```sh
curl -i http://localhost:8080/jobs \
  -H 'Content-Type: application/json' \
  -d '{"type":"log_analysis","input_key":"input/example.log"}'
```

For a completed job, request its download endpoint and confirm that it returns a short-lived presigned URL. The S3 object itself should remain private and direct anonymous access should fail.

## Troubleshooting

### `AccessDenied` from S3

Check all of the following:

- The EC2 instance has the `DJS-EC2-S3-Access` role attached.
- The policy uses `djs-s3-files-bucket`, not `YOUR_BUCKET_NAME`.
- `AWS_REGION` matches the bucket Region.
- Input keys begin with `input/`.
- Output keys begin with `output/`.
- The bucket does not have a restrictive explicit `Deny` policy.

### `S3_BUCKET is not set` or `AWS_REGION is not set`

Start the server from `backend/`, verify `backend/.env`, and restart the process.

### Public access warning

Remove policies using `"Principal": "*"`, keep all S3 Block Public Access settings enabled, and use the EC2 role or presigned URLs for application access.

