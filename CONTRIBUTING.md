# Contributing to PKV

Thank you for your interest in contributing to PKV!

## Getting Started

### Prerequisites
- Go 1.21 or later
- Bitwarden CLI (`bw`)
- Git

### Setting Up Development Environment

```bash
git clone https://github.com/shichao402/pkv.git
cd pkv
go mod download
make build
./pkv --help
```

## Development Workflow

1. **Fork the repository** on GitHub
2. **Clone your fork** locally
3. **Create a feature branch** from `main`:
   ```bash
   git checkout -b feature/your-feature-name
   ```
4. **Make your changes**
5. **Run tests and validation**:
   ```bash
   go vet ./...
   go build ./...
   ```
6. **Commit with clear messages**:
   ```bash
   git commit -m "feat: add new feature" -m "Description of changes"
   ```
7. **Push to your fork** and **create a Pull Request**

## Code Style

- Follow standard Go conventions (gofmt, goimports)
- Write clear, idiomatic Go code
- Add comments for exported functions and complex logic
- Keep functions focused and small

## Testing

Before submitting a PR:
- Build the project: `make build`
- Run linter: `go vet ./...`
- Test commands manually with your own Bitwarden vault
- Verify the install script works

## Reporting Issues

Please use GitHub Issues to report bugs or suggest features. Include:
- **Description** of the issue
- **Steps to reproduce** (for bugs)
- **Expected vs actual behavior**
- **Environment** (OS, Go version, etc.)
- **Logs or error messages** (if applicable)

## Release Process

PKV releases are fully driven by CI from a single source of truth (`version.json`).
**Do not create or push tags manually** — the `Auto Release` workflow creates the tag itself.

1. Bump `version.json` (no `v` prefix, e.g. `0.5.1`).
2. Add a `## [vX.Y.Z] - YYYY-MM-DD` section to `CHANGELOG.md`.
3. Commit: `git commit -m "chore(release): bump version to vX.Y.Z"`.
4. `git push origin main` — that is the entire release step.

CI (`.github/workflows/auto-release.yml`) will read `version.json`, create the tag,
cross-compile 6 platforms, and publish the GitHub Release.

Full details and recovery procedure (if a tag is pushed by mistake):
see [`RELEASE_CHECKLIST.md`](./RELEASE_CHECKLIST.md).

## Questions?

Feel free to open an issue with the question tag or start a discussion.

Thank you for contributing! 🎉
