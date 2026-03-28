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

1. Update version in code and documentation
2. Update CHANGELOG.md
3. Create a git tag: `git tag v0.2.0`
4. Push the tag: `git push origin v0.2.0`
5. GitHub Actions will automatically build and create a release

## Questions?

Feel free to open an issue with the question tag or start a discussion.

Thank you for contributing! 🎉
