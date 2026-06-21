# Contributing to leanmcp

Thanks for your interest in improving leanmcp! Contributions of all kinds are
welcome — bug reports, fixes, new compaction strategies, docs, and tests.

## Ground rules

- Be respectful. This project follows the [Code of Conduct](CODE_OF_CONDUCT.md).
- Keep changes focused. One logical change per pull request.
- Discuss large or breaking changes in an issue before opening a PR.
- All contributions are licensed under the project's [Apache-2.0](LICENSE) license.

## Project principles

leanmcp optimizes MCP tool-call responses. Keep these in mind when proposing changes:

- **Lossless by default.** Built-in compaction must never drop data without a recoverable
  handle. Lossy behavior (e.g. field projection) is opt-in and off by default.
- **No upstream API changes.** leanmcp is a transparent proxy; it must work with any MCP
  server without that server changing.
- **Fail open.** Any internal failure must return the full, correct result. Optimization
  is best-effort, never a correctness or availability risk.
- **Identity-scoped state.** Cached results are namespaced to a verified caller identity.

## Development workflow

1. Fork the repo and create a branch from `main`.
2. Make your change with tests (we follow test-driven development — write the failing
   test first, then the implementation).
3. Run the checks below and make sure they pass.
4. Open a pull request describing the change and the motivation.

## Local checks

```bash
go build ./...
go test ./...
go vet ./...
gofmt -l .        # should print nothing
```

If you add a dependency, run `go mod tidy` and commit the updated `go.mod`/`go.sum`.

## Commit messages

- Use [Conventional Commits](https://www.conventionalcommits.org/) prefixes:
  `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`.
- Write in the imperative mood ("add tabular compaction", not "added").
- Do not add `Co-Authored-By` trailers.

## Reporting bugs

Open an issue with: what you expected, what happened, the MCP client + upstream server
you used, and a minimal reproduction (a sample tool result that misbehaves is ideal).

## Security

Please do not file public issues for security problems. See [SECURITY.md](SECURITY.md).
