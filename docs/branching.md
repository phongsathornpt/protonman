# Branch and Merge Policy

`main` is Protonman's only long-lived development branch and must remain releasable.

Create short-lived branches from `main` using one of these prefixes: `feat/`, `fix/`, `perf/`, `refactor/`, `docs/`, `test/`, `ci/`, or `chore/`. Open pull requests back to `main`; do not use a permanent integration or `develop` branch.

Pull requests should contain one logical change, pass required CI and security checks, resolve review conversations, and receive the required maintainer or CODEOWNER approval for protected surfaces.

Squash merge is the preferred merge strategy so `main` retains one reviewable commit per pull request. Delete merged topic branches after merge. Force pushes and deletion of `main` are prohibited.

Release tags use `vMAJOR.MINOR.PATCH` (with an optional prerelease suffix) and are created only from commits already present on `main`. See `docs/releasing.md` for the release procedure.
