# Contributing to Protonman

Thanks for contributing to Protonman. Keep changes focused, tested, and easy to review.

## Development

Requirements: Go 1.27+, Git, and Make.

```sh
make lint
make test
make test-race
make build
```

Use short-lived branches from `main`: `feat/*`, `fix/*`, `perf/*`, `refactor/*`, `docs/*`, `test/*`, `ci/*`, or `chore/*`.

## Pull requests

- Open pull requests against `main`.
- Keep one logical change per pull request.
- Add or update tests for behavioral changes.
- Avoid unrelated formatting or refactors.
- Use a Conventional Commit style pull-request title, such as `fix(tui): handle ctrl-enter over ssh`.
- Resolve review conversations before merge.

Security-sensitive changes may require maintainer review. See `SECURITY.md` for vulnerability reporting.
