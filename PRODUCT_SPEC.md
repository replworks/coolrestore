# PRODUCT_SPEC.md

# coolrestore

## Purpose

coolrestore restores a previously created storage backup
archive into a specified directory on a server, so that files lost or
overwritten can be brought back exactly as they existed in the backup.

## Problem

A storage backup archive exists in S3-compatible storage or as a local
file, but there is no safe, predictable way to apply it back to a target
directory. Manually applying it risks writing to the wrong location,
silently overwriting files that should be kept, or applying content that
cannot be trusted to be well-formed.

## Product Goals

- Let an operator restore a backup archive into a target directory with a
  predictable, previewable outcome.
- Guarantee that no restore attempt can modify anything without the
  operator's explicit, separate confirmation.
- Guarantee that the target directory is unchanged for every failure before
  Change Application, and that replace-mode application failures are
  recovered through atomic rename rollback.
- Work the same way regardless of which service's storage is being
  restored.

## Users

- An operator who needs to restore application file storage after data
  loss, a bad deployment, or a server migration.
- An operator who needs to verify, before committing to a restore, exactly
  what a restore operation would do.

## Inputs

- **Archive source**: supplied once through `--source`, using either
  - an S3 URI in the form `s3://bucket/object-key`, or
  - an absolute path to a local archive file already present on disk.
- **Archive format**: a gzip-compressed tar archive (`.tar.gz`).
- **Target directory**: an absolute path on the local filesystem where the
  archive's contents should be restored.
- **Restore mode**: `merge` (default) or `replace`.
- **Confirmation flag**: `--confirm`, an explicit signal that authorizes
  real changes; its absence means no changes may be made.
- **Optional staging location**: a staging base directory supplied through
  `--staging`, or an operating-system temporary directory when omitted.
- **Optional integrity-check override**: `--skip-checksum`, a signal to
  skip the size-based integrity check before restoring the archive.
- **S3 access information**: supplied only through the environment variables
  defined in FRAMEWORK.md, never as direct credential input.

## Outputs

- A **plan report**, produced when no confirmation is given, describing:
  archive source, target directory, mode, and what would be restored.
- A **result report**, produced after a confirmed restore, describing:
  archive source, target directory, mode, and the number of files
  restored.
- A **failure report**, produced when a restore cannot proceed or does not
  complete, describing which step failed and confirming whether the target
  directory was left unchanged or may contain merge-mode partial changes.
- A **process exit status** indicating success or failure, suitable for
  use in automated scripts.

## Functional Requirements

1. The product must default to a plan-only run: unless the confirmation
   flag is explicitly given, no file under the target directory may be
   created, modified, or deleted.
2. The product must accept an archive source as either an S3-compatible
   location or a local file path.
3. The product must accept a target directory as an absolute path. If the
   target does not exist, plan-only execution must not create it; a
   confirmed execution may create it during Change Application. The final
   target path component must not be a symbolic link. Symbolic links in
   existing parent components may be resolved to their real directories
   before validation.
4. The product must support two restore modes:
   - `merge`: files present in the archive but not in the target are
     added; files present in both are overwritten with the archive's
     version; files present only in the target are left untouched and are
     never deleted.
   - `replace`: after the operation, the target directory contains exactly
     the archive's contents, and nothing else.
5. The product must refuse to run in `replace` mode unless the
   confirmation flag is also given.
6. The product must refuse to treat the following target directories as
   valid restore targets: an empty value, any filesystem root, and the
   well-known system-critical directories defined for the supported
   operating systems in FRAMEWORK.md. `/tmp` and its children are not
   system-critical targets by this rule.
7. The product must verify the archive's integrity before restoring any of
   its contents, unless the operator explicitly opts out of this check.
8. The product must reject, without restoring any part of the archive, any
   archive whose contents include:
   - an absolute path,
   - a path that would resolve outside the target directory (e.g. via
     `../`),
   - a symbolic link that would resolve outside the target directory,
   - a hard link,
   - any file type other than regular files and directories.
9. The product must confirm there is enough available space to complete
   archive acquisition and staging before making any change to the target
   directory, and must refuse to proceed if there is not. The calculation
   must include the uncompressed size of regular files and temporary
   staging artifacts. Replace-mode rollback uses atomic renames on the same
   filesystem and does not require a second data copy.
10. The product must not allow two restore operations to run against the
    same target directory at the same time; a second attempt while one is
    already running against that target must be rejected.
11. If any step of a confirmed restore fails before the archive's contents
    are applied to the target directory, the target directory must be left
    completely unchanged.
12. If a `replace`-mode restore fails while contents are being applied to
    the target directory, the product must restore the target directory's
    prior directory entry using the atomic rename rollback mechanism. A
    replace operation must be refused before mutation when that mechanism
    cannot be used, including when staging and target are on different
    filesystems.
13. On successful completion, the product must report the archive source,
    target directory, mode used, and the number of regular-file archive
    entries restored. Directories are not included in this count.
14. On any failure, the product must report which step failed and whether
    the target directory was left unchanged or may contain merge-mode
    partial changes.
15. The product must produce a non-zero process exit status on any
    failure, and a zero exit status only on success (including a
    successful plan-only run).
16. The product must accept a local archive file as a source without
    requiring any S3 access, so a restore can be exercised without
    connectivity to S3-compatible storage.

## User Flows

### Flow 1 — Preview a restore (default)

1. Operator specifies an archive source and a target directory, without
   the confirmation flag.
2. Product validates the inputs.
3. Product reports the plan: what would be restored, and in what mode,
   with no changes made.

### Flow 2 — Confirmed restore, merge mode (default mode)

1. Operator specifies an archive source, a target directory, and the
   confirmation flag.
2. Product validates the inputs, verifies archive integrity, checks
   available space, and validates archive contents for safety.
3. Product applies the archive's contents to the target directory in
   merge mode.
4. Product reports the result, including the number of files restored.

### Flow 3 — Confirmed restore, replace mode

1. Operator specifies an archive source, a target directory, the
   confirmation flag, and the replace mode.
2. Product performs the same validation as Flow 2.
3. Product replaces the target directory's contents with exactly the
   archive's contents.
4. Product reports the result, including the number of files restored.

### Flow 4 — Rejected restore due to unsafe archive content

1. Operator specifies an archive source and target directory, with or
   without the confirmation flag.
2. Product detects unsafe content in the archive (traversal, symlink,
   hardlink, or disallowed file type) during validation.
3. Product rejects the operation before any change to the target
   directory and reports which entries were rejected and why.

### Flow 5 — Restore attempted while one is already running

1. Operator starts a restore against a target directory.
2. While it is still running, a second operator (or the same operator)
   starts another restore against the same target directory.
3. The second attempt is rejected immediately, with a report that a
   restore is already in progress for that target.

## Error Conditions

- Archive source is missing, unreadable, or does not exist.
- Archive fails integrity verification.
- Archive is not a valid gzip stream.
- Archive is not a valid tar stream.
- Archive contains an absolute path, a path-traversal entry, an
  out-of-target symlink, a hard link, or a disallowed file type.
- Target directory is empty, unspecified, or a disallowed system path.
- `replace` mode is requested without the confirmation flag.
- Insufficient available space to complete the restore.
- A restore is already in progress for the same target directory.
- The archive source cannot be reached or read (e.g. S3 access failure).
- A confirmed restore fails partway through applying changes to the
  target directory.

For every error condition before Change Application, the product must make
no change to the target directory. A merge-mode failure during Change
Application may leave applied file changes; it must remove temporary
artifacts and report that the target may be partially changed. A
replace-mode failure during Change Application must use the atomic rename
rollback defined in requirement 12.

## Non-Goals

- The product does not create backups.
- The product does not restore databases.
- The product does not start, stop, or restart any application.
- The product does not inspect or validate application-level data
  correctness.
- The product does not manage or call any deployment platform's API.
- The product does not provide a graphical or web-based interface.
- The product does not prompt interactively for missing information; all
  required input must be supplied directly.

## Acceptance Criteria

- Given a valid archive and target, and no confirmation flag, running the
  product produces a plan report and makes no change to the target
  directory.
- Given a valid archive and target, and the confirmation flag, running the
  product in merge mode adds and overwrites the archive's files in the
  target directory and deletes nothing.
- Given a valid archive and target, and the confirmation flag with replace
  mode, running the product results in the target directory containing
  exactly the archive's contents.
- Given an archive containing a path-traversal or symlink-escape entry,
  running the product in any mode results in no change to the target
  directory and a rejection report.
- Given a target directory already undergoing a restore, a second restore
  attempt against the same target is rejected without affecting the
  first.
- Given an archive source that fails to download or read, the target
  directory is unchanged after the attempt.
- Given a forced failure partway through a replace-mode restore, atomic
  rename rollback restores the prior target directory entry.
- Given a successful restore, the reported file count matches the number
  of files actually present in the target directory that originated from
  the archive.

## Success Criteria

- An operator can determine exactly what a restore will do before
  committing to it, every time.
- No archive, regardless of its internal contents, can cause a change
  outside the specified target directory.
- No failure before Change Application changes the target directory, and a
  failed replace application restores the prior target directory entry
  through atomic rename rollback. Merge-mode application failures are
  reported as potentially partial and leave no temporary artifacts.
- The product behaves identically regardless of which service's storage
  is being restored.
