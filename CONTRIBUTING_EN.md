# Contributing Guide

Thank you for your interest in the Memoh-v2 project! We welcome all forms of contributions, including but not limited to:

- Submitting bug reports
- Requesting new features
- Submitting code fixes or new features
- Improving documentation
- Sharing usage experiences

## How to Contribute

### Reporting Bugs

If you find a bug, please report it via [GitHub Issues](https://github.com/91zgaoge/memoh-X/issues). Include the following information:

1. **Description**: A clear and concise description of the bug
2. **Steps to Reproduce**: List the specific steps to reproduce the bug
3. **Expected Behavior**: Describe what you expected to happen
4. **Actual Behavior**: Describe what actually happened
5. **Environment**: Operating system, Docker version, browser, etc.
6. **Screenshots or Logs**: If applicable, include relevant screenshots or error logs

### Requesting Features

If you have an idea for a new feature, feel free to submit it via [GitHub Issues](https://github.com/91zgaoge/memoh-X/issues). Please include:

1. **Feature Description**: A clear description of the feature you want
2. **Use Case**: Describe when and why this feature would be useful
3. **Possible Implementation**: If you have implementation ideas, feel free to share

### Submitting Code

1. **Fork the Repository**: Click the Fork button in the upper right corner
2. **Clone Your Fork**:
   ```bash
   git clone https://github.com/YOUR_USERNAME/memoh-X.git
   cd memoh-X
   ```
3. **Create a Branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```
4. **Develop**: Write your code
5. **Commit Changes**:
   ```bash
   git add .
   git commit -m "feat: add new feature description"
   git push origin feature/your-feature-name
   ```
6. **Create Pull Request**: Create a PR on GitHub to the main repository

## Code Standards

### Go Code Standards

- Use `gofmt` to format code
- Follow the [Effective Go](https://golang.org/doc/effective_go.html) guidelines
- Use meaningful names for functions and variables
- Exported functions and types need documentation comments
- Handle errors properly, don't ignore error return values

### TypeScript/Vue Code Standards

- Use ESLint to check code
- Component names use PascalCase
- Props and events should have meaningful names
- Complex logic needs comments

### Commit Message Convention

We use [Conventional Commits](https://www.conventionalcommits.org/) specification:

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types:**

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, semicolons, etc.)
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `test`: Test-related changes
- `chore`: Build process or auxiliary tool changes
- `ci`: CI/CD related changes

**Example:**

```
feat(memory): add memory compression feature

Implement LLM-based automatic memory compression mechanism that merges redundant information when there are too many memory entries.

Closes #123
```

## Development Environment Setup

### Prerequisites

- Docker and Docker Compose
- Go 1.25+ (for local backend development)
- Node.js 20+ (for local frontend development)
- Bun (for Agent Gateway development)

### Starting Development Environment

```bash
# Clone repository
git clone https://github.com/91zgaoge/memoh-X.git
cd memoh-X

# Start services
docker compose up -d

# Access Web UI
open http://localhost:8082
```

## Testing

Before submitting a PR, please ensure:

1. Code compiles successfully
2. Relevant test cases pass
3. No new lint errors are introduced

Run tests:

```bash
# Go tests
cd /data2/memoh-v2
go test ./...

# Frontend tests
cd packages/web
pnpm test
```

## Code Review

All Pull Requests need to be reviewed by at least one maintainer. During review, you may be asked to:

- Modify code style
- Add or modify tests
- Update documentation
- Explain design decisions

Please be patient and respond positively to review comments.

## License

By contributing code to this project, you agree that your contributions will be released under the [AGPL-3.0](LICENSE) license.

## Getting Help

If you encounter any issues during contribution, you can get help through:

- Start a discussion in [GitHub Discussions](https://github.com/91zgaoge/memoh-X/discussions)
- @ relevant maintainers in Issues

---

Thank you again for your contribution!
