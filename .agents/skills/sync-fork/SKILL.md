---
name: sync-fork
description: Sync an upstream Git repository into a customized fork on a development branch, resolve integration conflicts, verify retained features, and prepare a reviewed pull request. Use for fork synchronization and upstream merges; not for deployment or resetting a fork to upstream.
---

# Sync fork

Integrate upstream history while preserving the fork's custom behavior. Work on
the development branch requested by the user (default `develop`); keep the
release/default branch unchanged until its integration is authorized and verified.

For this two-repository workspace, read
[references/dramaclaw-workspace.md](references/dramaclaw-workspace.md).
Use repository-local instructions and current Git state over remembered SHAs,
remote names, test counts, or old screenshots.

## Establish the actual synchronization boundary

- Find the target repository root, read its `AGENTS.md`, contribution rules,
  PR template, CI workflows, and any more-specific instructions for edited files.
- Inspect working changes, current branch, local/remote branches, remote URLs,
  and recent history. Preserve unrelated changes. Do not silently stash, reset,
  overwrite an existing branch, or include unrelated files in a commit.
- `origin` normally names the user's fork; `upstream` normally names its direct
  source. These names are conventions, not evidence. Confirm the source from
  the user's request, fork metadata, and repository documentation. A fork of a
  fork may have a grandparent project: do not jump to that grandparent by default.
- Correct/add a remote only within the authorized sync scope. Fetch both fork
  and source, then record their exact SHAs and ahead/behind counts. Fetching does
  not update the working branch. Stop on fetch errors rather than using stale refs.
- List the upstream-only and fork-only changes and identify the custom features
  that need preservation. Counts from a screenshot are not a synchronization plan.

## Prepare and merge

1. Record the starting release SHA and create a uniquely named local checkpoint
   tag, such as `backup/before-upstream-<date>-<time>`. Do not overwrite a tag.
2. If the fork remote has advanced, incorporate it with a fast-forward where
   possible. If it has diverged, inspect the history before deciding how to
   integrate it. Do not reset the local branch to the remote.
3. Create `develop` from the agreed starting point, or inspect and reuse it if it
   already exists. Preserve its existing development commits. An isolated
   worktree is useful if unrelated local work prevents an in-place merge.
4. Merge the recorded source commit with `git merge --no-ff --no-commit <sha>`.
   Using a fixed SHA makes the integration reproducible. If it is already an
   ancestor, report that there is nothing new to merge; do not invent a change.
   For a no-op sync, ancestry and clean-state checks are normally sufficient;
   run functional checks if requested or if there is evidence of existing breakage.
   Do not create an empty PR when the development and release branches match.
5. Resolve conflicts by behavior, not by choosing an entire side. Check adjacent
   auto-merged code too: two features may reuse a route, export, schema, provider
   number, or UI entrypoint without producing a textual conflict.

Preserve identity and compatibility information: upstream attribution, licenses,
protocol IDs, model mapping, saved data, security checks, and custom launch or
release tooling. Do not introduce a new product feature merely to resolve a merge.

For UI features, trace **reachable entrypoint → component → data/API → asset →
submitted payload**. Files and imports surviving a merge do not prove the feature
is still reachable. Similarly, an accepted async task does not prove delivery.

Use `git diff --name-only --diff-filter=U` and `git diff --cached --check` before
committing. `git merge --abort` is for abandoning this active merge; it is not a
substitute for preserving pre-existing user changes.

## Verify the result

- Derive checks from changed areas, CI, and the repository's lockfiles. Use the
  pinned package manager/toolchain; do not upgrade dependencies as part of sync.
- Run builds/typechecks and the relevant custom-feature and upstream regression
  tests. Add tests when a conflict resolution combines behaviors that existing
  tests do not distinguish, such as direct-provider and gateway model discovery.
- Check independently built modules separately when the repository requires it.
  Verify that referenced static assets are packaged, not just present in source.
- Run repository lint, secret scanning, DCO/sign-off and other required gates
  before pushing. Read the PR template before writing commits and descriptions;
  this avoids discovering contribution requirements after publication.
- Classify failures with evidence: integration regression, pre-existing failure,
  environment limitation, or timeout. Use an isolated pre-merge worktree for
  baseline comparison when necessary. Never switch or reset the user's working
  branch to perform baseline tests.
- Reproduce and fix narrow integration regressions. Do not disable a gate, weaken
  an assertion, or mark a failure skipped simply to get a green result. Preserve
  the tested contract when making a test portable across OSes or line endings.
- After changes or a demonstrated environment correction, rerun the affected
  checks. Report full-suite results and targeted reruns separately; do not label
  targeted success as a clean full-suite run. Distinguish mocked checks from
  real API/UI/end-to-end verification. Do not run paid generation or deploy as an
  implicit part of synchronization.

## Publish within the authorized scope

- Stage only the intended files. Follow commit conventions and use
  `git commit --signoff` where DCO is required, with the existing configured Git
  identity. Do not modify identity or falsely attest human review.
- Verify that both the original fork tip and recorded upstream SHA are ancestors
  of the result. Confirm the release branch stayed at its recorded SHA and that
  the working tree contains no accidental edits.
- When pushing/PR creation is authorized, push the development branch to the
  user's fork and create/update a PR **in that fork**, targeting its release
  branch. Check for an existing PR first, preserve the repository template, and
  include AI-assistance disclosure where required.
- Keep a PR draft when CI or required acceptance remains incomplete. Record
  concrete unresolved checks and known pre-existing defects. Do not claim all
  integration checks passed while remote CI is still running or has failed.
- Do not merge the PR, publish artifacts, deploy, or restart services unless that
  action is authorized. Prior authorization in the current task is sufficient;
  do not ask again for routine authorized steps.
- Avoid rewriting published history. A one-time authorization to amend one
  published commit does not authorize future force pushes. If a scoped rewrite
  is explicitly approved, verify the expected branch tip, preserve the intended
  tree/parents, and use `--force-with-lease=<ref>:<expected-old-sha>`.

Finish with the source/result SHAs, branch and PR state, preserved custom features,
verification evidence, remaining limitations, and the checkpoint location. Do not
describe synchronization as complete on the release branch when only `develop`
has been updated.
