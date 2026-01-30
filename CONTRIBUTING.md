# Contributing to MCP Drill

Thank you for your interest in contributing to MCP Drill! This document provides guidelines for contributing.

## Code of Conduct

Please be respectful and constructive in all interactions.

## How to Contribute

### Reporting Bugs

1. Check [existing issues](https://github.com/bc-dunia/mcpdrill/issues) first
2. Create a new issue with:
   - Clear title and description
   - Steps to reproduce
   - Expected vs actual behavior
   - MCP Drill version
   - OS and Go version

### Suggesting Features

1. Check existing issues for similar suggestions
2. Create an issue with:
   - Clear use case
   - Proposed solution
   - Alternatives considered

### Pull Requests

1. **Fork** the repository
2. **Create** a feature branch:
   ```bash
   git checkout -b feature/amazing-feature
   ```
3. **Make** your changes:
   - Follow existing code style
   - Add tests for new functionality
   - Update documentation
4. **Test** your changes:
   ```bash
   go test ./...
   ```
5. **Commit** with clear messages:
   ```bash
   git commit -m 'Add amazing feature'
   ```
6. **Push** to your fork:
   ```bash
   git push origin feature/amazing-feature
   ```
7. **Open** a Pull Request

## Development Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/mcpdrill.git
cd mcpdrill

# Build
go build ./...

# Test
go test ./...

# Build Web UI
cd web/log-explorer && npm install && npm run build
```

## Code Guidelines

- Follow Go best practices and conventions
- Use `gofmt` for formatting
- Run `go vet` before committing
- Keep functions focused and small
- Write clear comments for complex logic
- Add tests for new functionality

## Pull Request Guidelines

- Keep PRs focused on a single change
- Include tests for new functionality
- Update documentation as needed
- Ensure all tests pass
- Respond to review feedback promptly

## Questions?

- Open a [Discussion](https://github.com/bc-dunia/mcpdrill/discussions)
- Check [Documentation](docs/index.md)

Thank you for contributing!
