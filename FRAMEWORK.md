# FRAMEWORK.md

# coolrestore

This document assumes PRODUCT_SPEC.md and ARCHITECTURE.md are correct and
final. It defines implementation constraints only — it eliminates
ambiguity about how the architecture is built, not what the architecture
is.

Where a technology decision had not actually been made prior to this
document, it is recorded below as a resolved decision (see Implementation
Rules, Integrity Verification), not invented silently.

---

## Implementation Rules

```text
Language: Go >= 1.27.1
```

Check https://go.dev/dl/ for the current stable release before
implementation begins. Pin the exact version in go.mod and in the
toolchain directive. If the CI runner's `setup-go` action does not yet
support the pinned version, fall back to:

```text
go install golang.org/dl/go1.27.1@latest
go1.27.1 download
```

```text
Standard library is preferred over third-party packages everywhere a
standard library facility exists.
```

```text
CLI argument parsing: standard library `flag` package.
```

Do not introduce a third-party CLI framework (e.g. cobra, urfave/cli).

```text
S3 access: AWS SDK for Go v2.
```

Do not implement a custom S3 client. Do not call the Coolify API for any
purpose, at any point.

```text
Archive processing: `archive/tar`, `compress/gzip` (standard library).
```

Do not use a third-party archive library. Do not shell out to a `tar`
binary.

```text
Target Exclusivity Lock: an exclusive-create lock file
(open with O_CREATE|O_EXCL, standard library `os` package).
```

Do not use a database, external lock service, syscall-level `flock`, or an
in-memory-only lock (the lock must be visible across separate process
invocations on the same host). Lock file location: a fixed system
temporary directory, named deterministically from the resolved absolute
target path (so the lock can be acquired even before the target directory
itself exists).

```text
Integrity Verification: size comparison only, not content-hash comparison.
```

Coolify's backup upload process does not produce or store a companion
checksum, and S3 ETag values cannot be relied on as content hashes for
objects that may have been uploaded via multipart upload. Therefore:

- For an S3 source: compare the number of bytes actually downloaded
  against the object's reported size (from the S3 metadata returned at
  download time).
- For a local source: compare the number of bytes actually read against
  the file's size as reported by the filesystem at the time of use.
- This detects a truncated or incomplete transfer. It does not detect,
  and is not capable of detecting, tampering or bit-level corruption that
  preserves the original byte count.
- `--skip-checksum` disables this size comparison. Its name is retained
  for continuity with earlier discussion, even though no checksum is
  computed; it is understood by the implementation as "skip integrity
  size-check."
- Structural validity of the archive (valid gzip stream, valid tar stream)
  is a separate check, performed during extraction, and is never skipped
  by `--skip-checksum`.

```text
Error handling: explicit returned errors only.
```

Do not panic for any expected failure condition (missing file, bad
archive, network failure, lock contention, etc.). Panics are reserved for
programmer errors only (e.g. nil dereference from a bug), never for
expected runtime conditions.

---

## Architectural Constraints

```text
Architecture Style: Single Binary CLI.
```

No backend service, no database, no web server, no daemon process, no
long-running background process. The application starts, performs one
restore or plan-only operation, and exits.

Each implementation package maps to one coherent group of adjacent
responsibilities from ARCHITECTURE.md (see Directory Structure). A package
may group the explicitly listed related components, but must not implement
responsibilities belonging to another component's boundary. In particular:

```text
Only the package implementing Change Application may write to the target
directory.
```

```text
Only the package implementing Restore Planner may decide what should
change; it must not itself perform any write.
```

```text
The package implementing Restore Planner must be callable, and its return
value must be identical, whether or not the invocation is authorized —
authorization must not be a parameter to this package's planning function.
```

The staging directory must be created fresh per invocation and must never
be the target directory or any ancestor/descendant of it. If a staging base
does not exist, the implementation creates the base and then creates a
unique child directory for the invocation. The resolved target and staging
paths must be compared after resolving existing parent symlinks.

Replace-mode application requires target and staging to be on the same
filesystem so the atomic rename rollback sequence can be used. A request
that cannot satisfy this condition must fail before target mutation.

---

## Directory Structure

Use:

```text
cmd/
    coolrestore/

internal/
    cli/          (Invocation Validator; flag parsing, validation, exit code)
    lock/         (Target Exclusivity Lock)
    source/       (Archive Acquisition: S3 and local; size-based integrity check)
    archive/      (Capacity Checker, Staged Extractor, Content Safety Validator)
    restore/      (Restore Planner, Change Application)
    config/       (environment variable access for S3 credentials only)

tests/
    (or *_test.go colocated with each package)
```

Avoid:

```text
pkg/
services/
helpers/
utils/
common/
shared/
```

Package boundaries follow ARCHITECTURE.md's component boundaries, not
convenience.

The restore invocation uses these standard-library `flag` options:

```text
--source          required; /absolute/path/archive.tar.gz or s3://bucket/key
--target          required; absolute target directory path
--mode            optional; merge (default) or replace
--confirm         optional authorization for Change Application
--staging         optional staging base directory
--skip-checksum   optional; skip only the size-based integrity comparison
```

The final target path component must not be a symbolic link. Existing parent
symbolic links may be resolved before target validation. A target that does
not exist is valid for a confirmed invocation and is created only by Change
Application; plan-only validation and planning never create it.

For target safety, the following paths are rejected as targets:

```text
POSIX:   /, /bin, /boot, /dev, /etc, /home, /lib, /lib64, /media, /mnt,
         /opt, /proc, /root, /run, /sbin, /srv, /sys, /usr, /var
macOS:   /System, /Library, /Applications, /Users, /Volumes
Windows: any volume root, %SystemRoot%, %ProgramFiles%,
         %ProgramFiles(x86)%, %ProgramData%, %SystemDrive%\Users,
         %SystemDrive%\Recovery, %SystemDrive%\System Volume Information
```

The filesystem root for every mounted volume is rejected. `/tmp` and its
children are permitted unless they match another protected path.

---

## Testing Rules

```text
Testing framework: Go standard testing package only.
```

Do not introduce a third-party testing or assertion library.

```text
Unit tests must not connect to real AWS S3 and must not require AWS
credentials to be present.
```

Abstract the S3 client behind an interface in `internal/source`; use a
fake implementation for unit-level tests.

```text
S3 is an EXTERNAL_BOUNDARY (per AGENTS.md). Its required live test target
is a locally run RustFS S3-compatible object storage server that this
project starts and controls for testing — never Amazon's own S3 service.
```

A fake or `httptest`-based stand-in satisfies unit-level tests only; it
does not satisfy AGENTS.md's E2E requirement for this boundary. The E2E
test performs a real upload and a real download against the locally run
RustFS server, through the same AWS SDK v2 code path used in production,
switched to that server only via the already-defined `AWS_ENDPOINT_URL`
and `AWS_S3_FORCE_PATH_STYLE` settings — no test-only code path. No AWS
account, real AWS credentials, or network access to Amazon's infrastructure
is required to satisfy this.

```text
No test may depend on real available-disk-space state of the test-runner
machine.
```

Abstract the capacity check behind an interface so tests can simulate
both sufficient and insufficient space.

Required test coverage (traceable to PRODUCT_SPEC.md Acceptance Criteria
and ARCHITECTURE.md Architectural Invariants):

- An end-to-end test performing a real upload and a real download against
  the locally run RustFS server, exercising the same acquisition
  code path used in production. This satisfies AGENTS.md's EXTERNAL_BOUNDARY
  E2E requirement for the S3 source; it is required, not optional.
- S3 and local source acquisition (unit-level, against the fake/abstracted
  client), including a size mismatch (simulated truncated download) being
  rejected.
- `--skip-checksum` bypassing the size comparison specifically, while
  structural (gzip/tar) validation still runs.
- Invalid gzip stream rejection; invalid tar stream rejection.
- Path traversal, absolute path, symlink-escape, and hardlink rejection —
  each individually, each asserted to leave the target directory
  unchanged.
- Insufficient staging space halting before extraction.
- Merge-mode plan and applied result: added, overwritten, and
  target-only-preserved paths.
- Replace-mode plan and applied result: exact content match afterward.
- Restore Planner producing an identical plan for an otherwise-identical
  unauthorized vs. authorized invocation (the core architectural
  invariant).
- An unauthorized invocation never invoking the Change Application
  package's mutating function, verified via a call-tracking fake.
- Lock contention: a second invocation against the same target while the
  first holds the lock is rejected.
- A forced failure during Change Application in replace mode results in
  the target directory being restored to its pre-application state.
- A forced failure at every pre-Change-Application step results in a
  byte-for-byte unchanged target directory.
- Successful run reports the correct restored/planned file count.
- Every failure path produces a non-zero process exit status; every
  success path (including a plan-only run) produces a zero exit status.

---

## Deployment Rules

```text
Distribution: GitHub Releases.
```

```text
Build targets: GOOS=linux GOARCH=arm64, GOOS=linux GOARCH=amd64.
CGO_ENABLED=0 (static binaries).
```

CI (GitHub Actions) must run, in this order:

```text
gofmt check
go vet
go test ./...
go build (both targets)
SHA256 checksum generation (for the build artifacts)
GitHub Release artifact upload
```

No runtime dependency beyond the static binary itself, Git (for source
checkout during CI), and network access to S3-compatible storage and
GitHub at run time.

---

## Configuration Rules

```text
No persistent configuration file.
```

All input is via CLI flags and environment variables. Do not introduce a
YAML/JSON/TOML config file for this tool.

Environment variables (S3 access only):

```text
AWS_ACCESS_KEY_ID
AWS_SECRET_ACCESS_KEY
AWS_REGION
AWS_ENDPOINT_URL
AWS_S3_FORCE_PATH_STYLE
```

If none of the above are set, fall back to the AWS SDK's default
credential provider chain.

The tool must not accept credential values through CLI flags or positional
arguments. Operators may use a protected external environment file and a
wrapper script to provide these variables for repeated restores. Repository
examples live under:

```text
examples/
    coolrestore.env.example
    coolrestore-wrapper.sh
```

Examples must contain placeholders only and must never contain real
credentials. When the credential provider chain cannot load credentials, the
failure report must include a concise wrapper hint without displaying any
credential value. Release distribution must make the same wrapper pattern
available to operators who obtain only the binary.

---

## Security Rules

```text
Credentials come only from environment variables or the AWS SDK default
credential provider chain.
```

Never accept a credential value as a CLI flag. Never write a credential to
a log line, even at `--verbose`.

```text
Never store credentials in: repository, source code, configuration files.
```

```text
Never auto-chown restored files. Never hardcode a UID or GID.
```

Restored files take on the UID/GID of the process that ran the restore;
this is documented behavior, not something the tool adjusts.

```text
Never trust file permission bits embedded in the archive as authoritative.
```

```text
Archive content that fails Content Safety Validation must never be logged
in a way that could be mistaken for an instruction (log path and rejection
reason as inert data only).
```

---

## Framework-Specific Conventions

- Each package listed in Directory Structure exposes the minimum surface
  needed by the package that calls it; internal helper types are
  unexported.
- Errors returned across a package boundary are wrapped with enough
  context to identify which architectural component produced them (for
  Result Reporter to compose the failure report defined in
  PRODUCT_SPEC.md), without leaking file-system paths outside the
  target/staging scope unnecessarily.
- No package may import `internal/restore`'s Change Application types
  from `internal/source`, `internal/archive`, or `internal/cli` — the
  dependency direction follows the System Flow in ARCHITECTURE.md, not the
  reverse.
- Concurrency primitives are used only for the Target Exclusivity Lock;
  the rest of the flow is sequential, matching the single-invocation
  lifecycle in ARCHITECTURE.md.

---

## Non-Goals

Do not implement:

- Laravel, PHP, or any application-framework dependency.
- Coolify API integration.
- Database restore of any kind.
- Application business-logic inspection or validation.
- Application stop/start/restart of any kind.
- A persistent configuration file format.
- Content-hash-based tamper or corruption detection — not achievable
  without a change to how Coolify produces backups, and therefore
  explicitly out of scope for this tool as currently specified.

---

## Architectural Invariants

```text
Default execution mode == plan-only (no authorization signal given).
```

```text
Filesystem mutation only occurs via Change Application, and only when an
authorization signal is present.
```

```text
--mode=replace requires the authorization signal.
```

```text
No archive path traversal, symlink escape, or hardlink is ever applied to
the target directory.
```

```text
Concurrent invocations against the same target directory are mutually
exclusive.
```

```text
A failed size-integrity check, a failed structural validation, or a failed
download never mutates the target directory.
```

```text
The plan computed by Restore Planner does not depend on the authorization
signal.
```

```text
Single Binary CLI; no daemon, no database, no external service dependency
other than S3.
```

Any implementation that violates these invariants is incorrect.
