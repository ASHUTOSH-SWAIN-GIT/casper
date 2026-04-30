# Casper Run — Audit Timeline

- **Proposal hash:** `08961743f5392e8e28acbd83299100cc81ff528a927e83c33a335d91e82c02fd`
- **Action type:** `rds_resize`
- **Instance:** `casper-testt` (region `ap-south-1`)
- **Resize:** `db.t4g.micro` → `db.t4g.small`
- **Started:** 2026-04-30T01:48:10Z
- **Ended:** 2026-04-30T01:59:07Z
- **Duration:** 10m57.625s
- **Total events:** 20
- **Terminal event:** `plan_completed`

---

## Lifecycle events

### 1. `proposed` — 01:48:10

- Action type: `rds_resize`
- DB instance: `casper-testt`
- Region: `ap-south-1`
- Current class: `db.t4g.micro`
- Target class: `db.t4g.small`
- Hash: `29b71912bd234f42…` (prev: ``)

### 2. `policy_evaluated` — 01:48:10

- Decision: **`allow`**
- Reason: safe in-family one-step upsize
- Hash: `14496a246eb9f618…` (prev: `29b71912bd234f42…`)

### 3. `plan_compiled` — 01:48:10

- Forward steps: 8
- Rollback steps: 4
- Hash: `164f1c8f3db07e7b…` (prev: `14496a246eb9f618…`)

### 20. `plan_completed` — 01:59:07

- Plan kind: `forward`
- Action type: `rds_resize`
- Hash: `764aec97c8331e57…` (prev: `368ddec17ea02198…`)

---

## Forward plan

Compiled from proposal `08961743f5392e8e…`. **8 steps** in the order below.

### Step 1 — `describe-pre` (`aws_api_call`)

> Describe instance to capture pre-state

- On failure: `abort`
- Status: **executed (done)**

- Service: `rds`
- Operation: `DescribeDBInstances`
- Params: `{"DBInstanceIdentifier":"casper-testt"}`

### Step 2 — `preconditions` (`verify`)

> Re-check preconditions against captured pre-state

- On failure: `abort`
- Status: **executed (done)**

**Verifies (against captured response from step `describe-pre`):**

Assertions:

- `DBInstances[0].DBInstanceIdentifier` eq `casper-testt`
- `DBInstances[0].DBInstanceStatus` eq `available`
- `DBInstances[0].PendingModifiedValues` empty
- `DBInstances[0].DBInstanceClass` eq `db.t4g.micro`

### Step 3 — `modify` (`aws_api_call`)

> Resize instance to target class

- On failure: `rollback`
- Status: **executed (done)**

- Service: `rds`
- Operation: `ModifyDBInstance`
- Params: `{"ApplyImmediately":true,"DBInstanceClass":"db.t4g.small","DBInstanceIdentifier":"casper-testt"}`

### Step 4 — `poll-modifying` (`poll`)

> Poll until status=modifying

- On failure: `rollback`
- Timeout: `2m`
- Status: **executed (done)**

**Polls:**

- Service: `rds`
- Operation: `DescribeDBInstances`
- Params: `{"DBInstanceIdentifier":"casper-testt"}`

- Until: path `DBInstances[0].DBInstanceStatus` eq `modifying`
- Interval: `5s`

### Step 5 — `poll-available` (`poll`)

> Poll until status=available

- On failure: `rollback`
- Timeout: `30m`
- Status: **executed (done)**

**Polls:**

- Service: `rds`
- Operation: `DescribeDBInstances`
- Params: `{"DBInstanceIdentifier":"casper-testt"}`

- Until: path `DBInstances[0].DBInstanceStatus` eq `available`
- Interval: `15s`

### Step 6 — `verify-class` (`verify`)

> Verify class equals target and no pending modifications

- On failure: `rollback`
- Status: **executed (done)**

**Verifies (after fresh API call):**

- Service: `rds`
- Operation: `DescribeDBInstances`
- Params: `{"DBInstanceIdentifier":"casper-testt"}`

Assertions:

- `DBInstances[0].DBInstanceClass` eq `db.t4g.small`
- `DBInstances[0].PendingModifiedValues` empty

### Step 7 — `wait-verification-window` (`wait`)

> Wait verification window before sampling success metric

- On failure: `rollback`
- Status: **executed (done)**

**Waits:** `5m`

### Step 8 — `verify-metric` (`verify`)

> Assert success metric meets threshold over verification window

- On failure: `rollback`
- Status: **executed (done)**

**Verifies (after fresh API call):**

- Service: `cloudwatch`
- Operation: `GetMetricStatistics`
- Params: `{"Dimensions":[{"Name":"DBInstanceIdentifier","Value":"casper-testt"}],"MetricName":"CPUUtilization","Namespace":"AWS/RDS","Period":60,"Statistics":["Average"],"Window":"5m"}`

Assertions:

- `Datapoints.avg` lte `80`

---

## Rollback plan

Compiled at the same time as the forward plan. **4 steps**. **Not executed** in this run because the forward plan completed successfully (or because the failure mode was abort, not rollback).

### Step 1 — `rollback-modify` (`aws_api_call`)

> Resize instance back to original class

- On failure: `abort`
- Status: **not executed** _(rollback was not invoked)_

- Service: `rds`
- Operation: `ModifyDBInstance`
- Params: `{"ApplyImmediately":true,"DBInstanceClass":"db.t4g.micro","DBInstanceIdentifier":"casper-testt"}`

### Step 2 — `rollback-poll-modifying` (`poll`)

> Poll until status=modifying

- On failure: `abort`
- Timeout: `2m`
- Status: **not executed** _(rollback was not invoked)_

**Polls:**

- Service: `rds`
- Operation: `DescribeDBInstances`
- Params: `{"DBInstanceIdentifier":"casper-testt"}`

- Until: path `DBInstances[0].DBInstanceStatus` eq `modifying`
- Interval: `5s`

### Step 3 — `rollback-poll-available` (`poll`)

> Poll until status=available

- On failure: `abort`
- Timeout: `30m`
- Status: **not executed** _(rollback was not invoked)_

**Polls:**

- Service: `rds`
- Operation: `DescribeDBInstances`
- Params: `{"DBInstanceIdentifier":"casper-testt"}`

- Until: path `DBInstances[0].DBInstanceStatus` eq `available`
- Interval: `15s`

### Step 4 — `rollback-verify-class` (`verify`)

> Verify class is back to original and no pending modifications

- On failure: `abort`
- Status: **not executed** _(rollback was not invoked)_

**Verifies (after fresh API call):**

- Service: `rds`
- Operation: `DescribeDBInstances`
- Params: `{"DBInstanceIdentifier":"casper-testt"}`

Assertions:

- `DBInstances[0].DBInstanceClass` eq `db.t4g.micro`
- `DBInstances[0].PendingModifiedValues` empty

---

## Step execution

### `describe-pre` (`aws_api_call`) — **done**

> Describe instance to capture pre-state

- Started: 01:48:10.064
- Ended: 01:48:10.533
- Duration: 470ms
- AWS calls: 1
  1. `rds.DescribeDBInstances` (request id `d7090906-7cb5-4e10-a139-2710d23b17bb`)

### `preconditions` (`verify`) — **done**

> Re-check preconditions against captured pre-state

- Started: 01:48:10.534
- Ended: 01:48:10.534
- Duration: 0s

### `modify` (`aws_api_call`) — **done**

> Resize instance to target class

- Started: 01:48:10.534
- Ended: 01:48:11.112
- Duration: 579ms
- AWS calls: 1
  1. `rds.ModifyDBInstance` (request id `2ae5eb38-0d08-44f7-9820-ea1f2510582a`)

### `poll-modifying` (`poll`) — **done**

> Poll until status=modifying

- Started: 01:48:11.113
- Ended: 01:48:16.499
- Duration: 5.386s
- AWS calls: 2
  1. `rds.DescribeDBInstances` (request id `78959049-356c-4173-9d10-5272eba61310`)
  2. `rds.DescribeDBInstances` (request id `457a5141-5e8a-4adb-b6e3-64a8666ab491`)

### `poll-available` (`poll`) — **done**

> Poll until status=available

- Started: 01:48:16.502
- Ended: 01:54:07.227
- Duration: 5m50.725s
- AWS calls: 24
  1. `rds.DescribeDBInstances` (request id `8e3628eb-e351-4206-82e1-912f4c6c6e6f`)
  2. `rds.DescribeDBInstances` (request id `20d51433-73c6-4c00-af0c-c637b02061ed`)
  3. `rds.DescribeDBInstances` (request id `dcfc4df5-860d-4873-91db-06c949999922`)
  4. `rds.DescribeDBInstances` (request id `aa449257-93b3-4421-af8f-ae873cb962e4`)
  5. `rds.DescribeDBInstances` (request id `a134c8e1-c1c4-4cf4-b523-453e26e327a6`)
  6. `rds.DescribeDBInstances` (request id `32c5369f-07fb-47ea-8c6e-680f96e30162`)
  7. `rds.DescribeDBInstances` (request id `6ebeb427-b260-43f3-81d4-46f2043a0d37`)
  8. `rds.DescribeDBInstances` (request id `1e576a8c-b16c-4ff1-ae2a-0ea0a6f806d1`)
  9. `rds.DescribeDBInstances` (request id `ee9d2b62-d98d-4c4f-bc36-b2aa8929e6d9`)
  10. `rds.DescribeDBInstances` (request id `5584783f-1303-4b42-b29f-ab5996675d05`)
  11. `rds.DescribeDBInstances` (request id `dfc7d3db-1afd-4cc3-84c8-fe9ddfccde0b`)
  12. `rds.DescribeDBInstances` (request id `3d9fba4d-73b1-4a03-9e6e-1d976f22eb0d`)
  13. `rds.DescribeDBInstances` (request id `775d520d-1ae5-49c3-b4ff-0d25e36169d7`)
  14. `rds.DescribeDBInstances` (request id `928e7dd1-045c-4c20-be23-db04540492bc`)
  15. `rds.DescribeDBInstances` (request id `fa38c8bd-f88b-4f15-804a-0018fdac7508`)
  16. `rds.DescribeDBInstances` (request id `ddf37f7d-64d4-415d-bddc-3cefdf270e15`)
  17. `rds.DescribeDBInstances` (request id `96f4b409-672c-42f9-a364-cfc3fa074926`)
  18. `rds.DescribeDBInstances` (request id `2da2e845-7433-4dfe-aa02-c82264d61d5b`)
  19. `rds.DescribeDBInstances` (request id `882e1f00-274b-4345-89f9-f4dce01d1962`)
  20. `rds.DescribeDBInstances` (request id `a87d5265-6e86-4c29-a586-87774d29e987`)
  21. `rds.DescribeDBInstances` (request id `2cb43045-2733-4e7d-8a0a-d7447ed2438d`)
  22. `rds.DescribeDBInstances` (request id `bfcff802-75e6-4120-a478-25ae84df2456`)
  23. `rds.DescribeDBInstances` (request id `74010e49-a803-4b6f-bbf8-463106849a6e`)
  24. `rds.DescribeDBInstances` (request id `5e480dbe-fbd2-42f5-97f2-bdb5adfb9e62`)

### `verify-class` (`verify`) — **done**

> Verify class equals target and no pending modifications

- Started: 01:54:07.238
- Ended: 01:54:07.402
- Duration: 164ms
- AWS calls: 1
  1. `rds.DescribeDBInstances` (request id `3401a976-a779-462a-8a1c-3acf8093dfc5`)

### `wait-verification-window` (`wait`) — **done**

> Wait verification window before sampling success metric

- Started: 01:54:07.403
- Ended: 01:59:07.402
- Duration: 5m0s

### `verify-metric` (`verify`) — **done**

> Assert success metric meets threshold over verification window

- Started: 01:59:07.403
- Ended: 01:59:07.684
- Duration: 281ms
- AWS calls: 1
  1. `cloudwatch.GetMetricStatistics` (request id `dd9f8c03-aeda-4663-8b07-dc0925f0093f`)

---

## Hash chain

- Genesis prev_hash: `` (should be empty)
- Genesis hash: `29b71912bd234f42…`
- Final hash: `764aec97c8331e57…`
- Chain length: 20 events

If `casperctl run` printed `audit log: N events, chain verified` to
stderr, every event's `prev_hash` matched the previous event's `hash`
and every event's `hash` matched `sha256(prev_hash || canonical(payload))`.
Tampering with any event's payload would have broken the chain forward
of that event.

