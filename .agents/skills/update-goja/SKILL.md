---
name: update-goja
description: Merge current dop251/goja master into Sobek while preserving Sobek's module support and Test262 coverage. Use for a new upstream update or a refresh of an existing goja merge branch.
---

# Update goja in Sobek

Sobek is a fork of goja with ESM support. Both projects use the Test262 runner in `tc39_test.go`; Sobek enables additional coverage, including module tests. Use a real merge commit so upstream ancestry remains visible; never rebase or squash the goja history.

## Prepare the merge

1. Read the repository's `AGENTS.md` and inspect the working tree. Keep unrelated work intact.
2. Check the existing remotes. The upstream remote is conventionally named `goja` and should point to `https://github.com/dop251/goja.git` (or `git@github.com:dop251/goja.git` for SSH). If it is missing, run `git remote add goja https://github.com/dop251/goja.git`; if the same repository already has another remote name, use that remote. Fetch current Sobek `main` and goja `master`, then verify their commit IDs. Do not rely on stale remote-tracking refs.
3. Review commits and upstream PRs since the last imported goja commit. Check relevant open Sobek issues and overlapping local PRs; record what each upstream change actually fixes and what remains unresolved.
4. Start the update from fetched `origin/main`. Merge the fetched goja master with `--no-ff`. If refreshing an in-progress branch, preserve its old commit for comparison and rebuild it only when the user wants the branch rewritten.

## Preserve Sobek behavior

- Resolve conflicts by understanding both sides. Keep Sobek's module loader, import/export behavior, and module tests unless the upstream change specifically requires a compatible adjustment.
- Inspect every change to `tc39_test.go`, especially Sobek's additional enabled tests, skip maps, feature blacklists, module handling, and dynamic imports. Do not remove a suite or add broad skips to make the merge pass. Remove an existing skip only after the behavior is implemented and its test passes.
- Compare the candidate tree with the previous merge when refreshing a branch. Explain any difference beyond the newly fetched upstream changes.

## Check coverage and behavior

- Use the same Test262 checkout before and after the merge. The repository's `.tc39_test262_checkout.sh` records the working fixture commit; verify the checkout rather than assuming `go test ./...` ran Test262, because `TestTC39` skips when the fixture is absent.
- Record Test262 pass, skip, and fail totals from `go test -json -count=1 -run '^TestTC39$' .` for both revisions. Compare the module-related suites separately even when the overall totals improve. Investigate any loss of executed tests or new skips before committing.
- Run relevant regression tests and `go test ./...`; check `go vet ./...`, `gofmt`, and the staged diff with `git diff --cached --check`. Keep upstream regression tests where they apply.

## Record and present the result

- Put the Sobek base SHA, goja master SHA, material conflict decisions, and coverage result in the merge commit message. Keep the two merge parents intact.
- In a PR description, link upstream PRs and affected Sobek issues. For a partly fixed issue, state the remaining behavior and avoid `Closes` or `Fixes`. Mention overlapping local PRs that the upstream merge supersedes.
- Follow the user's requested push and PR scope. If an existing PR's branch is rewritten and pushed, verify its head SHA and update any stale title or description.
