# DramaClaw workspace profile

This workspace contains independent Git repositories. The workspace root itself
may not be versioned. Shared skills under its `.agents/skills` are not automatically
included in either repository's commits or GitHub PRs. Check this before migrating
repository-owned skill files or tests outside their Git root.

The workspace root `.agents/skills` is the single source for shared skills. Both
`dramaclaw/.agents/skills` and `dramaclaw-gateway/.agents/skills` point to it. Edit
the root files; do not replace these directory links with copied skill folders.
On this Windows workspace these are directory junctions because the account lacks
symbolic-link privileges. Junctions are local filesystem configuration, not Git
symlink entries: Git does not preserve their target, and a standalone clone does
not carry the shared source. Do not publish duplicate skill contents accidentally
when staging repository changes; decide shared-source versioning separately.

## Source selection

| Checkout | User fork (`origin`) | Direct source (`upstream`) |
| --- | --- | --- |
| `dramaclaw` | `longlongago2/dramaclaw` | `dramaclaw/dramaclaw` |
| `dramaclaw-gateway` | `longlongago2/dramaclaw-gateway` | `dramaclaw/dramaclaw-gateway` |

Confirm these identities against the actual remotes and fork metadata. New API
(`QuantumNous/new-api`) is a further upstream of `dramaclaw-gateway`; synchronizing
the user's gateway fork does not imply merging directly from New API.

Use `develop` for integration and leave `main` unchanged until the release merge
is authorized and ready. Check whether `develop` and its PR already exist.

## DramaClaw checks

- Read `AGENTS.md`, `DESIGN.md` for visual/token changes, and CI/DCO requirements.
- Preserve MrToken discovery and H3/Seedance request semantics, local Gateway
  launch/billing setup, Harbor scripts and deployment parameters, and style assets.
- The style gallery previously survived in files but lost its normal image-form
  entrypoint. Verify reachability independently; do not claim an upstream sync
  repaired it merely because gallery component tests or image hashes pass.
- Pin frontend tooling from `frontend/package.json`; use frozen installation,
  production build and relevant Vitest tests. Test the Python route/provider,
  launch and publishing contracts for the changed features. No need to rebuild
  or deploy Docker images unless separately requested.
- Shared-route collision to watch: MrToken direct model discovery can accept
  unsaved credentials; the generic channel picker fetches saved gateway models.
  Preserve and test both request/response contracts rather than registering two
  handlers on the same route or discarding one side.

## Gateway checks

- Read root and `web/AGENTS.md`, CI, the PR template, and
  `docs/dc-media/upstream-sync-release.md`. Treat the latter's New API workflow as
  project-maintainer context, not permission to skip the direct source above.
- Preserve the New API Go module identity and attribution, DC protocol fields,
  optional zero/false values, task public/upstream ID separation, terminal states,
  signed result delivery, provider routing and custom channel numbers.
- Include MrToken native H3 image upload, mapped model identity, Seedance's distinct
  protocol, task polling/content proxy, and the local billing behavior in checks.
- Follow the repository's Go version and CI commands. Check root vet/build/tests
  and independently build/test `relaykit` with `GOWORK=off` where required.
  Root-module success alone does not establish independent module compatibility.
- Use the Bun version/commands from repository CI for `web`; read its instructions
  before touching React components. Build web assets when Go embeds `web/dist`.
  A temporary embed placeholder is sufficient only for the CI checks that specify
  one; do not present it as a production frontend build.
- Follow the PR template and required AI-assistance disclosure. Do not check
  human-authored or human-reviewed attestations on the user's behalf.

## Windows environment lessons

These are diagnostics, not automatic global configuration changes:

- If the `uv` executable's trampoline fails, try `python -m uv` or the existing
  project interpreter. Do not silently recreate its virtual environment.
- Tests invoking POSIX shell scripts may require Git's `bin` and `usr/bin` on the
  **test process** PATH. Diagnose missing `sh` before treating it as a regression.
- Python subprocess tests may need `PYTHONUTF8=1` to read UTF-8 PowerShell output.
- Package runners can occasionally invoke a shell shim as JavaScript. Inspect the
  installed package's declared entrypoint and call it with the intended runtime;
  do not change project dependencies to work around a launcher error.
- Source-text assertions must tolerate LF/CRLF when line endings are irrelevant.
  Preserve semantic assertions. Extend a test timeout only after confirming that
  environment load, not a functional defect, explains the failure.
- Avoid heavy builds competing with timeout-sensitive UI tests. Use bounded
  concurrency and record test flags/environment corrections in the report.
