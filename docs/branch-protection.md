# Branch Protection Setup

After merging the CI workflow PR, enable branch protection on `main` so pull requests cannot merge unless lint and tests pass.

## Steps

1. Open **GitHub → Repository → Settings → Branches**
2. Click **Add branch protection rule** (or edit existing rule for `main`)
3. Branch name pattern: `main`
4. Enable:
   - **Require a pull request before merging**
   - **Require status checks to pass before merging**
   - **Require branches to be up to date before merging** (recommended)
5. In **Status checks that are required**, search and select:
   - `lint`
   - `test`
6. Save changes

## What this enforces

| Check | Workflow job | Fails when |
|-------|--------------|------------|
| `lint` | `.github/workflows/ci.yml` → `lint` | golangci-lint reports issues |
| `test` | `.github/workflows/ci.yml` → `test` | tests fail or coverage &lt; 80% |

## Local verification

```bash
golangci-lint run ./...

CGO_ENABLED=1 HERMES_SESSION_SECRET="local-dev-secret-at-least-32-bytes" \
  ./scripts/check-coverage.sh
```

## Notes

- Public repos get unlimited GitHub Actions minutes for this workflow.
- Coverage gate excludes `cmd/server` and `internal/testutil` (see `scripts/check-coverage.sh`).
- Adjust threshold via `COVERAGE_THRESHOLD=80` environment variable.
