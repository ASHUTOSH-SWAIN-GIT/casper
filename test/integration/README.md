# Integration tests

Tests in this folder hit **real AWS** and require credentials. They are
gated by the `integration` build tag and skipped from the default
`go test ./...` run.

## Run

```bash
# Default unit test run (everything in this folder is excluded):
go test ./...

# Run integration tests only:
go test -tags=integration ./test/integration/...
```

## Required environment

| Var | Purpose |
|---|---|
| `AWS_REGION` | Region the test RDS instance lives in |
| `AWS_PROFILE` *or* standard creds | IAM identity Casper assumes from |
| `CASPER_TEST_DB_INSTANCE` | Identifier of a throwaway RDS instance |
| `CASPER_TEST_TARGET_CLASS` | Instance class to resize *to* (e.g. `db.t4g.medium`) |

The instance pointed at by `CASPER_TEST_DB_INSTANCE` will be modified
and rolled back. **Never point this at anything that holds real data.**

## What lives here

Integration tests are the proof that the interpreter, the AWS client
implementation, and the action contract work together against the real
AWS API. Unit tests next to source (`internal/.../*_test.go`) cover
logic in isolation; these cover the seams.
