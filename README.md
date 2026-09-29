# Cool Restore

[![CI](https://github.com/replworks/coolrestore/actions/workflows/ci.yml/badge.svg)](https://github.com/replworks/coolrestore/actions/workflows/ci.yml)
[![release](https://github.com/replworks/coolrestore/actions/workflows/release.yml/badge.svg)](https://github.com/replworks/coolrestore/actions/workflows/release.yml)
[![update-changelog](https://github.com/replworks/coolrestore/actions/workflows/update-changelog.yml/badge.svg)](https://github.com/replworks/coolrestore/actions/workflows/update-changelog.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/replworks/coolrestore.svg)](https://pkg.go.dev/github.com/replworks/coolrestore)
[![Go Version](https://img.shields.io/github/go-mod/go-version/replworks/coolrestore)](https://github.com/replworks/coolrestore)
![License](https://img.shields.io/github/license/replworks/coolrestore)

> Restore Coolify storage safely after an incident.
>
> Preview the change first. Apply it only with explicit confirmation.

coolrestore restores gzip-compressed tar archives created from Coolify storage and stored in S3-compatible storage such as RustFS. It supports local archive files and S3 objects, merge restores, replace restores, isolated staging, and atomic replace rollback.

The archive is validated and staged before anything is written to the target.
By default, coolrestore only produces a plan.

---

## Installation

### GitHub Release (Recommended)

Download the release binary for the target server:

```text
coolrestore-linux-amd64
coolrestore-linux-arm64
```

Install it as `coolrestore` somewhere on the server's `PATH`:

```bash
install -m 0755 coolrestore-linux-amd64 /usr/local/bin/coolrestore
```

Verify the binary:

```bash
file /usr/local/bin/coolrestore
```

Release binaries are statically built for Linux and macOS on `amd64` and
`arm64`; Windows archives are also published.

### Homebrew Cask

Install the published cask from the Replworks tap:

```bash
brew tap replworks/tap
brew install --cask coolrestore
```

Upgrade it later with:

```bash
brew upgrade --cask coolrestore
```

The cask provides prebuilt binaries for macOS and Linux on `amd64` and
`arm64`. Homebrew downloads the binaries from the published GitHub Release.
The current macOS binary is not signed or notarized by Apple, so the cask uses
a macOS-only post-install hook to remove the download quarantine attribute.
This is a distribution workaround, not a replacement for code signing and
notarization; future releases should move to Apple Developer ID signing and
notarization and then remove the hook.

### Go

For development or local builds:

```bash
go install github.com/replworks/coolrestore/cmd/coolrestore@latest
```

Verify the installed binary version:

```bash
coolrestore --version
```

Show the available commands and restore options:

```bash
coolrestore help
# or
coolrestore --help
```

---

## Configuration

S3-compatible access uses the AWS SDK environment variables:

```bash
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export AWS_REGION=...
export AWS_ENDPOINT_URL=https://rustfs.example.com
export AWS_S3_FORCE_PATH_STYLE=true
```

If these variables are not set, the AWS SDK default credential chain is used.
Credentials are never accepted as command-line flags.

For package installations, an explicit environment file can be selected
without depending on the install path:

```bash
cat > ./coolrestore.env <<'EOF'
AWS_ACCESS_KEY_ID=replace-me
AWS_SECRET_ACCESS_KEY=replace-me
AWS_REGION=us-east-1
AWS_ENDPOINT_URL=https://s3-compatible.example.com
AWS_S3_FORCE_PATH_STYLE=true
EOF
chmod 600 ./coolrestore.env
```

Use it explicitly when running a restore:

```bash
coolrestore \
  --env-file ./coolrestore.env \
  --source 's3://bucket/object-key.tar.gz' \
  --target /tmp/coolrestore-test
```

The file uses simple `KEY=VALUE` entries. Blank lines and full-line comments
are allowed; shell commands and `export` statements are not evaluated.
`./coolrestore.env` is never auto-discovered.

For repeated restores, keep `coolrestore.env` protected and pass it explicitly:

```bash
install -m 600 ./coolrestore.env /etc/coolrestore/env
coolrestore \
  --env-file /etc/coolrestore/env \
  --source 's3://bucket/object-key.tar.gz' \
  --target /tmp/coolrestore-test
```

For a user-specific setup, use `~/.config/coolrestore/env` and pass that path
with `--env-file`.

---

## Usage

Diagnose an S3 source before restoring. This is read-only: it checks the
environment configuration, credentials, endpoint, and object metadata
without downloading the archive body or requiring a target directory:

```bash
coolrestore diagnose \
  --env-file ./coolrestore.env \
  --source 's3://coolify-bucket/data/coolify/backups/volumes/replworks-team-0/wjpmbrff7txw1hqtwu4jb4xb/directory-datawifinotestorage-1790266116.tar.gz'
```

For a local archive, diagnosis checks that the path exists and is readable:

```bash
coolrestore diagnose \
  --source /backups/coolify-storage.tar.gz
```

List available S3 backup archives below a prefix without downloading them:

```bash
coolrestore list \
  --env-file ./coolrestore.env \
  --source 's3://coolify-bucket/data/coolify/backups/volumes/replworks-team-0/wjpmbrff7txw1hqtwu4jb4xb/'
```

Select the newest `.tar.gz` archive below a prefix for a restore:

```bash
coolrestore \
  --env-file ./coolrestore.env \
  --source 's3://coolify-bucket/data/coolify/backups/volumes/replworks-team-0/wjpmbrff7txw1hqtwu4jb4xb/' \
  --latest \
  --target /var/lib/coolify/storage
```

Preview a local archive. Preview is the default and does not modify the target directory:

```bash
coolrestore \
  --source /backups/coolify-storage.tar.gz \
  --target /var/lib/coolify/storage
```

Use an S3-compatible object as the source:

```bash
coolrestore \
  --env-file ./coolrestore.env \
  --source s3://coolify-backups/storage/2026-09-26.tar.gz \
  --target /var/lib/coolify/storage
```

Apply a merge restore explicitly:

```bash
coolrestore \
  --source s3://coolify-backups/storage/2026-09-26.tar.gz \
  --target /var/lib/coolify/storage \
  --mode merge \
  --confirm
```

Replace the target with the archive contents:

```bash
coolrestore \
  --source s3://coolify-backups/storage/2026-09-26.tar.gz \
  --target /var/lib/coolify/storage \
  --mode replace \
  --confirm
```

Use a specific staging base when required:

```bash
coolrestore \
  --source /backups/coolify-storage.tar.gz \
  --target /var/lib/coolify/storage \
  --staging /var/lib/coolify/restore-staging
```

Optional integrity override:

```bash
coolrestore \
  --source /backups/coolify-storage.tar.gz \
  --target /var/lib/coolify/storage \
  --skip-checksum
```

`--mode replace` requires `--confirm`. The target must be an absolute path and must not be a protected system directory or a final symbolic link.

---

## Restore Modes

### Merge

Merge mode adds files that are missing from the target and overwrites files with the same path. Files that exist only in the target are preserved.

### Replace

Replace mode makes the target contain exactly the archive's staged contents.
The target and staging location must be on the same filesystem so the change can use atomic directory renames and rollback.

---

## Example

### Preview

```text
source: s3://coolify-backups/storage/2026-09-26.tar.gz
target: /var/lib/coolify/storage
mode: merge
outcome: planned
regular_files: 2
added:
  uploads/avatar.png
overwritten:
  config/app.php
```

### Successful restore

```text
source: s3://coolify-backups/storage/2026-09-26.tar.gz
target: /var/lib/coolify/storage
mode: merge
outcome: restored
regular_files: 2
```

### Failed restore

```text
source: s3://coolify-backups/storage/2026-09-26.tar.gz
target: /var/lib/coolify/storage
mode: merge
outcome: failed
step: change application
target_state: may contain partial changes
```

The process exits with `0` for a successful preview or restore and a non-zero status for failures.

---

## Safety Model

coolrestore is designed for infrastructure recovery, not application-level validation.

- Archive paths are checked for traversal and absolute paths.
- Symlinks, hard links, and unsupported archive entry types are rejected.
- Archive contents are extracted into an isolated staging directory first.
- Staging and target paths may not overlap.
- A target lock prevents concurrent restores against the same directory.
- Merge failures clean temporary artifacts but may retain changes already applied.
- Replace failures restore the previous target directory through atomic rename rollback.

The backup archive produced by Coolify is treated as an input object. This tool does not create or require a SHA256 manifest for that archive. SHA256 files attached to GitHub Releases, when present, verify the downloaded coolrestore binary itself.

---

## Troubleshooting

### S3 object could not be read

Verify:

- `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`
- `AWS_REGION`
- `AWS_ENDPOINT_URL` for RustFS or another S3-compatible service
- `AWS_S3_FORCE_PATH_STYLE=true` when required by the endpoint
- bucket and object-key permissions

If credentials cannot be loaded, coolrestore prints a short `--env-file` hint. It never prints credential values. Use a protected file such as `/etc/coolrestore/env` and pass it explicitly with `--env-file`.

### Replace mode was rejected

Replace mode requires explicit authorization:

```bash
--mode replace --confirm
```

It also requires staging and target to be on the same filesystem.

### The target was not changed during preview

This is expected. Omit `--confirm` to inspect a plan without changing the target. A missing target directory is not created during preview.

### The archive was rejected

Check the failure report for the archive entry and reason. Unsafe archives are rejected before any content is applied to the target.

---

## Development

Run tests:

```bash
go test ./...
```

Run static checks:

```bash
go vet ./...
git diff --check
```

Build the local binary:

```bash
go build ./cmd/coolrestore
```

Build release targets:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o coolrestore-linux-amd64 ./cmd/coolrestore
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o coolrestore-linux-arm64 ./cmd/coolrestore
```

---

## License

See the repository license before distributing the tool.

---

Built for safe Coolify storage recovery after an incident.
