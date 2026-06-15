# Agent PR Review Rules

Use this repository-wide guide when checking GitHub PRs with `gh`.

## Preferred workflow

1. List open PRs with the key merge fields:
   ```bash
   gh pr list --state open --limit 20 \
     --json number,title,author,isDraft,mergeStateStatus,mergeable,reviewDecision,statusCheckRollup,url
   ```

2. Inspect an individual PR with `gh pr view` before deciding it is mergeable:
   ```bash
   gh pr view <number> --json \
     number,title,url,headRefName,baseRefName,mergeStateStatus,mergeable,reviewDecision,reviewRequests,latestReviews,statusCheckRollup
   ```

3. Use `gh pr checks <number>` for a concise pass/fail summary.

4. If a check fails, drill into the failing workflow with:
   ```bash
   gh run view <run_id> --log-failed
   ```

## Mergeability rules of thumb

- `mergeable: MERGEABLE` is not enough on its own.
- `mergeStateStatus: CLEAN` is the best sign that GitHub considers the PR ready.
- `mergeStateStatus: BLOCKED` can still happen even when checks are green; check review requirements, repository policy, or other non-CI gates.
- For dependency bumps in `services/merrymaker-go/frontend`, make sure the lockfile changes are included. CI runs `bun install --frozen-lockfile`, so a package.json-only bump will fail.

## Mend checks

- Do **not** wait for Mend checks to finish.
- In this repository, Mend checks are expected to never complete and should not be used as a blocking signal.
- When reviewing PR readiness, rely on GitHub merge state, required checks, and human/policy gates instead.

## Practical reminder

When a PR looks green but still cannot merge, re-check:

- `mergeStateStatus`
- `reviewDecision`
- `reviewRequests`
- failing required checks in `statusCheckRollup`

That combination is more reliable than any single check name.
