# Repository Guidelines
##
Use Chinese 

## Project Structure & Module Organization
- `vsftp-exporter.go` contains the exporter entry point plus collectors, SSH helpers, and metric registration; keep new packages close to their domain logic before splitting to subdirectories.
- `config.json` is the canonical example config; add new settings with sane defaults and document them in both this file and `README.md`.
- `grafana-dashboard.json` tracks the default dashboard; update it alongside metric changes and note revisions in pull requests.
- `log/` is a scratch space for sample FTP logs when developing parsers; do not commit production data.

## Build, Test, and Development Commands
- `go mod tidy` aligns dependencies after adding or removing imports.
- `go fmt ./...` formats Go files; run it before sending patches.
- `go build -o vsftp-exporter vsftp-exporter.go` compiles the exporter binary.
- `go run vsftp-exporter.go -config=./config.json` starts the exporter against the sample configuration; adjust paths for remote log tests.
- `go test ./...` executes unit and integration tests once they are added.

## Coding Style & Naming Conventions
Follow idiomatic Go style enforced by `gofmt`. Use descriptive CamelCase for exported types and methods, lowerCamelCase for internals, and prefix Prometheus metrics with `vsftp_` to stay consistent. Keep functions small and focused on either log parsing, metric mutation, or transport concerns, and prefer streaming readers over loading entire files.

## Testing Guidelines
Write table-driven tests with Go’s `testing` package and use subtests to capture different log scenarios. When parsing logs, craft fixtures under `log/fixtures` (create as needed) and load them via `t.TempDir()` copies so the originals remain untouched. Target coverage for new features above 80%, and ensure tests run cleanly with `go test -race ./...` before merging.

## Commit & Pull Request Guidelines
Match the existing Conventional Commit pattern (`feat:`, `fix:`, `chore:`, optionally with Chinese scopes such as `feat(监控): ...`). Reference related issues in the message body when applicable. Pull requests should describe the change, include reproduction or validation steps, and attach updated screenshots when Grafana panels move. Confirm CI (or local `go test ./...`) results before requesting review.

## Configuration & Security Tips
Never commit real credentials; use placeholders in `config.json` and document secret handling in the PR. When introducing new configuration keys, add validation in the config loader and communicate defaults in the README. For remote monitoring, prefer SSH key authentication and document any required firewall or log-retention changes alongside the code.
