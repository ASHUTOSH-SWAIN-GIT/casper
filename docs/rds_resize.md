# Action: RDS Instance Resize

**Action type:** `rds_resize`
**AWS API:** `rds:ModifyDBInstance` (change `DBInstanceClass`)
**Reversibility class:** `reversible` (resize back to original instance class)

This is the first and only action Casper supports in v1. Every layer downstream — JSON Schema, plan compiler, simulator inputs, policy rules, identity-broker session policy, verifier — is derived from this document. If any of those disagree with this spec, this spec wins.

---

## 1. Intent

Change the compute size of a single RDS instance, on demand, in response to load (typically CPU pressure). Storage, engine version, parameter groups, networking, and Multi-AZ topology are out of scope for this action.

---

## 2. Inputs

The proposal must specify:


| Field                                  | Type     | Notes                                                                                                                                                          |
| -------------------------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `db_instance_identifier`               | string   | The RDS instance to modify. Resource ARN is derived from this + the connected account.                                                                         |
| `region`                               | string   | AWS region the instance lives in (e.g. `us-east-1`).                                                                                                           |
| `current_instance_class`               | string   | The instance class observed at proposal time (e.g. `db.r6g.large`). Recorded for predictability — at execution time, the actual current class must match this. |
| `target_instance_class`                | string   | The instance class to resize to (e.g. `db.r6g.xlarge`).                                                                                                        |
| `apply_immediately`                    | bool     | `true` for v1 — schedule-window-only resizes are deferred. The simulator must surface that `true` causes a brief interruption.                                 |
| `success_criteria.metric`              | string   | CloudWatch metric used for verification. v1: `CPUUtilization`.                                                                                                 |
| `success_criteria.threshold_percent`   | number   | Pass condition: metric average over the verification window is ≤ this value (for CPU-pressure resizes).                                                        |
| `success_criteria.verification_window` | duration | How long after the instance returns to `available` we sample the metric. v1 default: 5m.                                                                       |
| `reasoning`                            | string   | Free-text rationale from the proposer (LLM or human). Stored for audit; not used for control flow.                                                             |


The proposal does **not** carry credentials, role ARNs, or any identity material. Identity is the broker's job, scoped per execution.

---

## 3. Preconditions (checked before any write call)

The interpreter checks each as an explicit step. Failure of any precondition aborts before the modify call.

1. **Instance exists.** `DescribeDBInstances` returns exactly one instance with the given identifier in the given region.
2. **Instance is available.** `DBInstanceStatus == "available"`. Any other status (`modifying`, `backing-up`, `rebooting`, `creating`, `deleting`, `failed`, …) aborts.
3. **No pending modifications.** `PendingModifiedValues` is empty. A pending change means a prior `ModifyDBInstance` is queued; resizing on top of it is ambiguous.
4. **Current class matches the proposal.** `DBInstanceClass == current_instance_class`. If it doesn't, the proposal was built against stale state — refuse, don't guess.
5. **Target differs from current.** `target_instance_class != current_instance_class`. A no-op resize is not an action.
6. **Target is valid for this engine.** `DescribeOrderableDBInstanceOptions` lists `target_instance_class` for the instance's engine + engine version. Catches typos and engine-incompatible classes (e.g. memory-optimized class on an unsupported engine version).
7. **Instance is not Multi-AZ-promoting / read-replica-creating.** Read from `DescribeDBInstances` flags. (Edge case; surfaces as a generic "instance is in a transient state" abort if hit.)

The simulator runs the same precondition checks at proposal time (read-only). If a precondition fails at simulation, the proposal cannot be approved. Re-checking at execution is mandatory — state may have drifted between approval and execution.

---

## 4. Observable effects (what changes, where to look)

When `ModifyDBInstance` with `ApplyImmediately=true` succeeds:

- `DBInstanceStatus` transitions: `available` → `modifying` → `available`. Typical duration: 5–15 minutes for a same-family resize, longer for cross-family.
- `PendingModifiedValues.DBInstanceClass` is briefly populated, then cleared once applied.
- A short connection interruption occurs as the new instance is brought up. For Multi-AZ instances the interruption is failover-shaped (~1 minute). For Single-AZ it is downtime-shaped (several minutes).
- An RDS event of type `db-instance` with category `notification` and message describing the modification is recorded.
- CloudTrail records `ModifyDBInstance` against the IAM identity used (the per-action session). The AWS request ID is the cross-reference key.

Nothing else should change. If anything else does (storage, parameter group, security group, etc.), the modification was malformed and verification must fail.

---

## 5. Plan shape (forward)

Compiled `ExecutionPlan` for a forward resize, in order:

1. `aws_api_call` — `DescribeDBInstances` → capture full pre-state (instance class, status, multi-az, engine, version, pending values).
2. `decision` — re-check all preconditions against the captured pre-state. Abort with structured error on any failure.
3. `aws_api_call` — `ModifyDBInstance(DBInstanceClass=target, ApplyImmediately=true)`. Idempotency: AWS does not provide an idempotency token for this call; we rely on the precondition that `current_instance_class` matches and `PendingModifiedValues` is empty to prevent double-apply.
4. `poll` — `DescribeDBInstances` until `DBInstanceStatus == "modifying"`. Timeout: 2m. (Confirms the modify took effect and we're not racing.)
5. `poll` — `DescribeDBInstances` until `DBInstanceStatus == "available"`. Timeout: 30m.
6. `verify` — assert `DBInstanceClass == target_instance_class` and `PendingModifiedValues` is empty.
7. `wait` — `success_criteria.verification_window` (e.g. 5m). Time is recorded by the step.
8. `verify` — query CloudWatch `GetMetricStatistics` for `success_criteria.metric` over the verification window; assert average ≤ `success_criteria.threshold_percent`.

Steps 7–8 are how we honor the "verification is a step" rule. A failure at step 6 or step 8 triggers the rollback plan automatically.

---

## 6. Rollback plan

A parallel `ExecutionPlan` produced at compile time, not at failure time:

1. `aws_api_call` — `ModifyDBInstance(DBInstanceClass=current_instance_class, ApplyImmediately=true)`.
2. `poll` — `DescribeDBInstances` until `DBInstanceStatus == "modifying"`. Timeout: 2m.
3. `poll` — `DescribeDBInstances` until `DBInstanceStatus == "available"`. Timeout: 30m.
4. `verify` — assert `DBInstanceClass == current_instance_class` and `PendingModifiedValues` is empty.

Rollback **does not** re-check the success metric. The point of rollback is to return the instance to its prior topology, not to re-validate the original problem.

If rollback fails (verification mismatch, AWS error, timeout), the proposal terminates in `rollback_failed`. This is a distinct end state from `done` and `rolled_back`; it is a serious incident that must be visible in the UI and should never be silently retried.

---

## 7. Failure modes

Categorized so the policy engine, simulator, and verifier can each handle them appropriately.

### Pre-execution failures (caught by preconditions)

- Instance not found, wrong region.
- Instance in non-`available` status.
- Pending modifications already queued.
- Stale `current_instance_class` (proposal built against drifted state).
- Target class invalid for engine.
- No-op (target == current).

### Execution failures (caught during the modify or polling)

- `InvalidDBInstanceState` returned by `ModifyDBInstance` (race with another modification).
- `InsufficientDBInstanceCapacity` — AWS cannot allocate the requested class right now.
- `InvalidParameterCombination` — class incompatible with current settings (storage type, IOPS).
- AWS throttling (`Throttling`, `RequestLimitExceeded`). Surfaces as step failure; SDK retries are disabled by design.
- Network / DNS errors talking to the AWS API.
- Polling timeout exceeded (instance stuck in `modifying` longer than 30m).

### Verification failures (caught after execution)

- Final `DBInstanceClass` does not equal `target_instance_class` (unexpected — would indicate AWS-side anomaly or a misread).
- `PendingModifiedValues` is non-empty after returning to `available` (another modification was queued concurrently).
- Success-criteria metric did not meet the threshold over the verification window — the resize "worked" mechanically but did not solve the intended problem.

### Rollback failures (caught during rollback execution)

- Same shapes as execution failures, applied to the rollback `ModifyDBInstance` call.
- Treated as a terminal `rollback_failed` state.

Every failure produces a structured error in the audit log: category, AWS error code (if any), AWS request ID (if any), the step it occurred in, and the captured request/response.

---

## 8. Success post-conditions

The action is `done` if and only if **all** of the following are true at the end of the plan:

1. `DescribeDBInstances` reports `DBInstanceStatus == "available"`.
2. `DBInstanceClass == target_instance_class`.
3. `PendingModifiedValues` is empty.
4. The CloudWatch success metric, averaged over the verification window starting from the moment the instance returned to `available`, is at or below `success_criteria.threshold_percent`.
5. The full audit log for the proposal hash-chains cleanly from `proposed` to `done` with no skipped state transitions.

If any of (1)–(4) is false, the rollback plan runs. (5) is a property of the audit layer, not of the action itself, but it is a hard precondition for declaring "this action succeeded" to the user.

---

## 9. Out of scope (v1)

To prevent the action from acquiring ad-hoc surface area:

- Storage modifications (size, type, IOPS).
- Engine version upgrades.
- Parameter or option group changes.
- Multi-AZ topology changes.
- Snapshot / restore-based resizes.
- Cross-region or cross-account moves.
- Read-replica resizes (the action targets primaries only in v1).
- Aurora cluster resizes (RDS instance only).
- Scheduled maintenance-window-only resizes (`apply_immediately=false`).

Each of the above is a candidate future action with its own spec. None should be quietly subsumed into this one.

---

## 10. Open questions

- Should `success_criteria` support metrics other than `CPUUtilization` in v1? Inclination: no — narrow first, prove the verifier shape works, generalize later.
- Should the proposal carry a max acceptable interruption duration, surfaced to the user before approval? Probably yes — the simulator can predict it from Multi-AZ status, but encoding it in the proposal makes it part of what's signed.
- For Multi-AZ instances, do we want to explicitly assert "interruption was failover-shaped" via RDS events as part of verification? Nice-to-have, not blocking v1.

