# ARCHITECTURE.md

# coolrestore

This document is technology-agnostic. It describes how the product defined
in PRODUCT_SPEC.md works internally: components, responsibility
separation, information flow, and invariants. It contains no
implementation technology, frameworks, languages, libraries, deployment
details, or coding conventions.

This document is written for AI implementation, not for human
documentation.

---

## Purpose

Define the internal structure of a system that turns a backup archive and
a target directory into either a plan (no change) or an applied restore
(a change), such that:

- the decision of _what would change_ and the act of _actually changing
  it_ are made in exactly one place each, and the same decision logic
  produces both the plan report and the applied result;
- the target directory can never be affected by anything in the archive
  that was not explicitly validated as safe;
- failures before Change Application never affect the target, and
  replace-mode application failures recover through atomic rename rollback.

---

## Core Concepts

- **Plan Before Act**: Every invocation computes what would change. Only
  when explicitly authorized does the system act on that computation.
  These are two different steps, not two modes of the same step.
- **Single Source of Truth for Change**: Exactly one component decides
  what the target directory's contents should become. No other component
  may independently derive or duplicate that decision.
- **Untrusted Payload**: Archive contents are never trusted. Every path,
  link, and file type inside it must be validated before it can influence
  the change decision at all.
- **Staged Reflection**: Archive contents are only ever examined from an
  isolated location. The target directory is never used as a workspace.
- **Exclusive Target Access**: Only one invocation may be active against a
  given target directory at a time, whether that invocation will end in a
  plan or an applied change.
- **Preserve on Failure**: If an attempt fails before application, the
  target directory remains unchanged. If a replace application fails,
  the prior target directory entry is restored by atomic rename rollback.

---

## System Flow

```
Invocation Validation
        ↓
Target Exclusivity Lock
        ↓
Archive Acquisition (remote or local)
        ↓
Integrity Verification
        ↓
Capacity Check (staging location)
        ↓
Content Safety Validation
        ↓
Staged Extraction
        ↓
Restore Planning (reads target, staged content, and mode; writes nothing)
        ↓
Authorization Check  ← only proceeds past here if the invocation was
        ↓                explicitly authorized to apply changes
Change Application (executes exactly the computed plan)
        ↓
Cleanup (staging, temporary artifacts)
        ↓
Lock Release
        ↓
Result Reporting
```

An unauthorized invocation follows this same flow through Restore
Planning, then reports the plan and stops. Change Application is never
reached.

---

## Components

The components below are logical responsibility boundaries. The framework
may group adjacent pre-application components into one implementation
package, but grouping must not transfer responsibility across the stated
boundaries.

### 1. Invocation Validator

- **Inputs**: The raw parameters of a single invocation (archive source
  description, target directory, mode, authorization signal, optional
  staging location, optional integrity-check override).
- **Outputs**: A validated, normalized invocation description, or an
  explicit rejection naming which parameter failed validation.

### 2. Target Exclusivity Lock

- **Inputs**: The validated target directory.
- **Outputs**: An acquired-lock handle, or an explicit "already in
  progress" rejection.

### 3. Archive Acquisition

- **Inputs**: The validated archive source description (remote location or
  local path) and, when remote, whatever access information the
  environment provides for reaching it.
- **Outputs**: A locally available archive artifact, or an explicit
  acquisition failure.

### 4. Integrity Verifier

- **Inputs**: The acquired archive artifact and the integrity-check
  override setting.
- **Outputs**: A pass/fail verification result.

### 5. Capacity Checker

- **Inputs**: The verified archive artifact and the intended staging
  location.
- **Outputs**: A pass/fail capacity result.

### 6. Staged Extractor

- **Inputs**: The safety-validated archive artifact and a staging location.
- **Outputs**: A fully extracted staging directory, or an explicit
  extraction failure.

### 7. Content Safety Validator

- **Inputs**: The verified archive artifact.
- **Outputs**: A pass/fail safety result, itemizing any rejected entries
  by path and reason.

### 8. Restore Planner

- **Inputs**: The safety-validated staged content, a read-only view of the
  current target directory, and the selected mode.
- **Outputs**: A change plan: for merge mode, the set of paths to add and
  the set of paths to overwrite (target-only paths are explicitly excluded
  from the plan); for replace mode, the complete set of paths the target
  should contain afterward. The plan also carries the counts needed for
  reporting.

### 9. Change Application (Reflection)

- **Inputs**: A change plan produced by the Restore Planner, and an
  explicit authorization signal.
- **Outputs**: An updated target directory matching the plan exactly, a
  confirmation of the number of paths affected, or an explicit application
  failure together with the target directory's recovered state.

### 10. Result Reporter

- **Inputs**: Either a change plan (unauthorized invocation) or the
  outcome of Change Application (authorized invocation), or a failure
  produced by any earlier component.
- **Outputs**: A plan report, a result report, or a failure report as
  defined in PRODUCT_SPEC.md, together with a process exit status.

---

## Component Responsibilities

1. **Invocation Validator** — Confirm every invocation parameter is
   present, well-formed, and internally consistent (e.g. mode/authorization
   combinations) before any other component runs. This is the only
   component responsible for rejecting a structurally invalid invocation.

2. **Target Exclusivity Lock** — Guarantee that no two invocations act on
   the same target directory concurrently, regardless of whether either
   invocation is a plan-only or authorized run. Responsible for its own
   release under every possible exit from the flow.

3. **Archive Acquisition** — Make the archive's bytes available locally,
   regardless of whether they originate remotely or locally. Responsible
   for reporting acquisition failure without ever having touched the
   target directory.

4. **Integrity Verifier** — Establish that the acquired archive is the
   archive that was intended to be restored, before any of its contents
   are interpreted or extracted. Responsible for honoring an explicit
   opt-out without silently skipping verification otherwise.

5. **Capacity Checker** — Establish, ahead of time, that extraction can
   complete without exhausting available space at the staging location.
   Responsible for treating insufficient space as a hard stop.

6. **Staged Extractor** — Turn a safety-validated archive into a fully materialized
   directory tree, without ever writing outside the isolated staging
   location.

7. **Content Safety Validator** — Establish, entry by entry, that nothing
   in the archive would escape, subvert, or misuse the target directory if
   it were staged or applied. Responsible for a complete, itemized rejection
   when unsafe content is found, not a partial pass.

8. **Restore Planner** — Be the single place where "what should change"
   is decided. Responsible for producing an identical plan whether the
   invocation will ultimately be authorized or not — the plan must not
   depend on the authorization signal. Responsible for never reading the
   target directory in a way that could be mistaken for, or turn into, a
   write.

9. **Change Application** — Execute the plan it is given, exactly as
   given, without recomputing or second-guessing what should change.
   In replace mode it is responsible for atomic rename rollback if
   execution fails partway through. In merge mode it is responsible for
   cleaning temporary artifacts and reporting that applied changes may be
   partial; complete merge rollback is not guaranteed.

10. **Result Reporter** — Translate whatever the flow produced (a plan, an
    applied result, or a failure) into the reports and exit status defined
    in PRODUCT_SPEC.md. Responsible for never altering the substance of
    what it reports.

---

## Responsibility Boundaries

- **Invocation Validator** owns structural correctness of inputs only. It
  never touches the archive, the target directory, or the lock.
- **Target Exclusivity Lock** owns only the lock's lifecycle. It has no
  opinion on what the invocation will do once it holds the lock.
- **Archive Acquisition**, **Integrity Verifier**, **Capacity Checker**,
  **Staged Extractor**, and **Content Safety Validator** collectively own
  everything that happens _before_ a change decision exists. None of them
  may write to, or read for decision-making purposes, the target
  directory.
- **Restore Planner** owns the change decision exclusively. No other
  component — including Change Application — may independently decide
  which paths are added, overwritten, or removed. Restore Planner reads
  the target directory but never writes it.
- **Change Application** owns target directory mutation exclusively. It is
  the only component permitted to write to the target directory, and it
  may only do so when both a change plan and an authorization signal are
  present. It has no authority to deviate from the plan it was given.
- Merge-mode failure recovery is limited to cleaning temporary artifacts;
  the target may contain changes already applied before the failure.
- In replace mode, Change Application must use an atomic rename sequence:
  move the existing target to a temporary backup name, move the staged
  result into the target name, and restore the backup name if the second
  rename or subsequent finalization fails. This sequence is valid only
  when target and staging are on the same filesystem.
- **Result Reporter** owns communication of outcomes only. It has no
  authority to alter, retry, or reinterpret the outcome it is given.
- Authorization is a single boolean fact carried by the invocation from
  the Invocation Validator onward. Only the boundary between Restore
  Planner and Change Application checks it; every earlier component is
  indifferent to it, which is what guarantees an unauthorized invocation
  and an authorized one see identical behavior up through planning.

---

## Data Flow

The archive flows one-directionally through acquisition, verification,
capacity checking, safety validation, and isolated extraction. At no point
in this chain is the target directory read or written.

At Restore Planning, the target directory is read for the first and only
time before a possible mutation, solely to compute overlaps for merge mode
or the full replacement set for replace mode. This read produces the
change plan, which is the single object that flows onward.

Change Application consumes only the change plan and the authorization
signal — it does not re-read the staged content or re-derive the plan from
the target directory. This ensures the plan that was reported (if an
unauthorized run had occurred) and the plan that is applied (in an
authorized run) are, structurally, the same computation.

Result Reporter consumes whichever of {change plan, applied outcome,
failure} the flow terminated with, and produces the corresponding report.
It does not consume raw archive or target directory data directly.

---

## Architectural Rules

- No component other than Change Application may write to the target
  directory.
- No component other than Restore Planner may decide what should change.
- Restore Planning must be reachable, and must complete, on every
  invocation that passes Content Safety Validation — regardless of
  authorization. Authorization only gates what happens _after_ planning.
- The Target Exclusivity Lock must be held from before Archive Acquisition
  begins until after Result Reporting completes, on every invocation,
  authorized or not.
- No component may proceed past a failed verification, safety check, or
  capacity check by substituting a partial or best-effort result.
- The staging location must be isolated such that a failure during
  extraction or planning cannot have already partially affected the
  target directory.

---

## Failure Boundaries

- **Invocation validation failure**: halts before the lock is acquired;
  target untouched.
- **Lock acquisition failure**: halts before archive access; target
  untouched.
- **Archive acquisition failure**: halts before extraction; target
  untouched; lock released.
- **Integrity verification failure**: halts before extraction; target
  untouched; lock released.
- **Capacity check failure**: halts before extraction; target untouched;
  lock released.
- **Content safety validation failure**: halts before Staged Extraction;
  target untouched; lock released.
- **Extraction failure**: halts before Restore Planning; staging cleaned
  up; target untouched; lock released.
- **Restore Planning failure** (e.g. the target directory cannot be read):
  halts before any authorization check; target untouched; lock released.
- **Unauthorized invocation reaching the end of Restore Planning**: this is
  not a failure; it is reported as a plan; Change Application is never
  reached.
- **Change Application failure (partway through mutation)**: in replace
  mode, the atomic rename rollback must restore the prior target directory
  entry before cleanup and lock release. Replace must be rejected before
  mutation if the same-filesystem precondition is not met. In merge mode,
  temporary artifacts must be cleaned and the failure report must state
  that the target may contain partial changes.

---

## Non-Goals

- This architecture does not define how backups are created.
- This architecture does not define any application-level validation of
  restored content's correctness.
- This architecture does not define multi-target or batch coordination;
  each invocation addresses exactly one target directory.
- This architecture does not define any long-running or event-driven
  process; each invocation is a single, bounded execution that ends at
  Result Reporting.

---

## Architectural Invariants

```text
Exactly one component (Restore Planner) decides what should change.
```

```text
Exactly one component (Change Application) is permitted to change the
target directory, and only when given both a plan and authorization.
```

```text
The plan computed for an unauthorized invocation and the plan executed for
an otherwise-identical authorized invocation are the same computation.
```

```text
No archive entry that fails Content Safety Validation can ever reach
Restore Planning or Change Application.
```

```text
At most one invocation holds the Target Exclusivity Lock for a given
target at any time.
```

```text
Any failure prior to Change Application leaves the target directory
exactly as it was found.
```

```text
Replace-mode application either completes through the atomic rename
sequence or restores the prior target directory entry before reporting
failure.
```

```text
The Target Exclusivity Lock is released on every exit path, without
exception.
```

Any implementation that violates these invariants is architecturally
incorrect, independent of whether it otherwise "works."
