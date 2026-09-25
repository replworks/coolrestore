# TASKS.md

## coolrestore

This document defines what remains to be implemented. It is an execution checklist, organized into phases. Each task represents a user-visible capability or an architectural milestone, is independently completable, and is independently testable against objective acceptance criteria. It does not describe architecture, technologies, frameworks, or implementation details.

A phase is complete only when every task in it satisfies its acceptance criteria.

Execute exactly one task at a time, in document order. Do not begin a later
task until the current task's implementation, tests, and acceptance criteria
are complete.

---

### Phase 1 — Invocation Safety Baseline

- [x] **Task 1.1 — Reject invalid restore requests before touching any data**
  - **Acceptance Criteria**
    - Given any invalid or incomplete combination of restore parameters (missing target, missing source, requesting full replacement without explicit authorization, etc.), the operation ends with a failure and no local or remote resource has been accessed.
    - Given a valid combination of parameters, the operation proceeds past this stage.
- [x] **Task 1.2 — Default every restore attempt to preview-only**
  - **Acceptance Criteria**
    - Given any valid restore request without explicit authorization, no file under the target directory is created, modified, or deleted, regardless of source, mode, or archive contents.
    - The absence of authorization never needs to be stated more than once; it is the default behavior of every request.
- [x] **Task 1.3 — Prevent concurrent operations against the same target**
  - **Acceptance Criteria**
    - Given a restore operation already in progress against a target, a second operation started against the same target is rejected immediately and does not affect the first operation's progress or outcome.
    - Given two operations against two different targets, both proceed independently and neither is rejected because of the other.

---

### Phase 2 — Archive Acquisition and Basic Verification

- [x] **Task 2.1 — Obtain the archive regardless of where it is stored**
  - **Acceptance Criteria**
    - Given a valid remote archive location, the archive is retrieved and made available for the rest of the operation.
    - Given a valid local archive path, the archive is used directly without requiring any remote access.
    - Given an invalid or unreachable source of either kind, the operation fails and the target directory is unaffected.
- [x] **Task 2.2 — Detect incomplete or malformed archive transfers**
  - **Acceptance Criteria**
    - Given an archive that was only partially retrieved, the operation fails before the archive's contents are examined further, and the target directory is unaffected.
    - Given a file that is not a valid compressed archive, the operation fails with a report identifying that the archive itself is invalid, and the target directory is unaffected.
- [x] **Task 2.3 — Confirm sufficient space before proceeding**
  - **Acceptance Criteria**
    - Given a case where the available space at the working location is insufficient for the archive's contents, the operation fails before any extraction occurs, and the target directory is unaffected.
    - Given sufficient available space, the operation proceeds past this stage.

---

### Phase 3 — Archive Content Safety

- [x] **Task 3.1 — Keep archive extraction isolated from the restore target**
  - **Acceptance Criteria**
    - At no point during extraction does any archive content appear under the target directory; this holds even when the operation ultimately fails during or after extraction.
- [ ] **Task 3.2 — Reject archives containing unsafe content**
  - **Acceptance Criteria**
    - Given an archive containing an entry that would resolve outside the target directory, act as a link leaving the target directory, act as a hard link, or introduce a disallowed file type, the entire operation is rejected before anything is applied to the target directory.
    - The failure report identifies which entries were rejected and why.
    - Given an archive where every entry is safe, the operation proceeds past this stage.

---

### Phase 4 — Restore Planning

- [ ] **Task 4.1 — Determine what a restore would change, without changing anything**
  - **Acceptance Criteria**
    - Given a safety-approved archive, a target, and a selected mode, requesting a preview produces a description of what would be added and/or overwritten (merge) or the complete resulting contents (replace), and results in zero changes to the target directory.
- [ ] **Task 4.2 — Guarantee the preview matches what execution would do**
  - **Acceptance Criteria**
    - For the same archive, target, and mode, the set of paths described by an unauthorized preview and the set of paths actually affected by an otherwise-identical authorized execution are identical.

---

### Phase 5 — Restore Execution

- [ ] **Task 5.1 — Perform a merge restore**
  - **Acceptance Criteria**
    - Given authorization and merge mode, every file present in the archive exists in the target afterward with the archive's content.
    - Every file that existed only in the target beforehand still exists, unchanged, afterward.
    - No file is deleted from the target as a result of this operation.
- [ ] **Task 5.2 — Perform a replace restore**
  - **Acceptance Criteria**
    - Given authorization and replace mode, the target directory's contents after the operation are exactly the archive's contents — nothing more, nothing less.
- [ ] **Task 5.3 — Recover safely when execution is interrupted**
  - **Acceptance Criteria**
    - Given a forced interruption during a replace-mode restore, the
      atomic rename rollback restores the prior target directory entry
      before the operation reports failure; the target is never left in a
      mixed or partial replace state.
    - Given a forced interruption during a merge-mode restore, all staging,
      temporary, and backup artifacts are cleaned, and the failure report
      states that changes already applied to the target may remain.

---

### Phase 6 — Operator Reporting

- [ ] **Task 6.1 — Report a clear, consistent outcome for every invocation**
  - **Acceptance Criteria**
    - Every invocation, regardless of outcome, produces a report that identifies the archive source, the target, the mode, and the outcome (planned, restored, or failed).
    - A successful preview and a successful execution are each reported with the affected file count.
    - A failure is reported with the reason and whether the target was left
      unchanged or may contain merge-mode partial changes.
- [ ] **Task 6.2 — Produce an automation-friendly result signal**
  - **Acceptance Criteria**
    - Every invocation that ends in success (including a successful preview) produces a result signal an automated caller can use to confirm success.
    - Every invocation that ends in failure produces a distinguishable result signal, without exception.

---

### Phase 7 — Distribution Readiness

- [ ] **Task 7.1 — Make the tool runnable in its intended operating environment**
  - **Acceptance Criteria**
    - An operator can obtain a runnable copy of the tool for the server environments it is intended to run on, without needing a local development setup to build it themselves.
    - The tool runs correctly in that environment using only what is already present on a typical target server plus network access to the archive source.

---

### Future

- [ ] **Task F.1 — Verify archive authenticity, not just completeness**
  - **Status**: Deferred — depends on the archive source eventually supplying a trustworthy reference (a checksum or manifest) that does not exist today.
  - **Acceptance Criteria**
    - When a trustworthy reference for a given archive is available, the operation detects content tampering or corruption that a simple size comparison would miss, in addition to the checks already in place.
    - Archives for which no such reference is available continue to be handled exactly as before; this capability does not change behavior for them.
