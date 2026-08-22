# Contributing to Axiom Clinic Appointment System

Thank you for your interest in contributing to Axiom! This document outlines our development workflow, coding standards, and contribution process.

---

## 🌿 Git Workflow

### Branch Strategy

| Branch | Purpose | Protection |
|--------|---------|------------|
| `main` | Production-ready releases | 🔒 Protected — squash merge, linear history, 1 approval, CI required |
| `dev` | Integration branch | 🔒 Protected — 1 approval, CI required, no linear history required |
| `feature/*` | New features | ✅ Push freely |
| `fix/*` | Bug fixes | ✅ Push freely |
| `chore/*` | Maintenance, deps, config | ✅ Push freely |
| `docs/*` | Documentation | ✅ Push freely |
| `refactor/*` | Code refactoring | ✅ Push freely |

### Branch Naming

```
feature/jwt-auth
fix/redis-connection-leak
chore/update-dependencies
docs/api-spec-update
refactor/auth-middleware
```

### Commit Message Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

**Types:** `feat`, `fix`, `refactor`, `chore`, `docs`, `test`, `perf`, `build`, `ci`

**Examples:**
```
feat(auth): add JWT access/refresh token rotation
fix(redis): fix connection leak in pool
refactor(auth): extract token validation to middleware
docs: update API documentation for appointments
test(auth): add unit tests for token refresh
```

---

## 🔄 Development Workflow

### 1. Starting Work

```bash
# Ensure you're on dev and up to date
git checkout dev
git pull origin dev

# Create feature branch
git checkout -b feature/your-feature-name

# Make changes, commit frequently
git add .
git commit -m "feat(auth): add login endpoint"

# Push and create PR
git push origin feature/your-feature-name
# Open PR on GitHub: feature/your-feature-name → dev
```

### 2. Pull Request Process

1. **Branch**: `feature/*` or `fix/*` → `dev`
2. **PR Template**: Fill out completely
3. **Review**: At least 1 approval required
3. **CI**: All checks must pass (`build`, `test`)
4. **Merge**: Squash merge to `dev` (linear history not required for `dev`)

### 3. Releasing to Production

1. Create PR: `dev` → `main`
2. **Squash merge** (linear history enforced)
3. Tag release: `git tag v1.0.0 && git push origin v1.0.0`
4. GitHub Actions builds and publishes Docker image

---

## 🔧 Hotfix Process

For urgent production fixes:

```bash
# 1. Branch from main
git checkout main
git pull origin main
git checkout -b fix/critical-bug-name

# 2. Make minimal fix, test thoroughly
git commit -m "fix(auth): prevent token replay attack"

# 3. Open PR to main
git push origin fix/critical-bug-name
# PR: fix/critical-bug-name → main

# 4. After merge to main, backport to dev
git checkout dev
git pull origin dev
git cherry-pick <commit-hash-from-main>
git push origin dev
# PR: dev backport → dev
```

**Rules:**
- Hotfix branches **only from `main`**
- PR to `main` first, then backport to `dev`
- Minimal scope — fix only the critical issue

---

## 💻 Development Setup

### Prerequisites
- Go 1.22+
- Docker + Docker Compose
- `golang-migrate` for migrations
- `sqlc` for code generation

### Commands

```bash
# Start dependencies
docker-compose up -d

# Run migrations
make migrate-up

# Run server
make run

# Run tests
make test          # standard
make test-race     # with race detector
make test-coverage # coverage report

# Code quality
make lint          # golangci-lint
make fmt           # gofmt + goimports

# Database
make migrate-up    # apply migrations
make migrate-down  # rollback last
make migrate-create NAME=name  # create new migration

# Generate sqlc code
make sqlc-generate
```

---

## ✅ Code Standards

### Go Style
- Follow [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- `gofmt` / `goimports` enforced via `make fmt`
- `golangci-lint` enforced via `make lint`

### Testing
- **Unit tests**: `*_test.go` alongside code
- **Integration tests**: Separate package, run with `make test`
- **Race detector**: Always run `go test -race ./...` before PR
- **Coverage**: Aim for >80% on business logic

### Error Handling
- Wrap errors with context: `fmt.Errorf("context: %w", err)`
- Sentinel errors for expected cases: `errors.Is(err, ErrNotFound)`
- Never ignore errors — handle or propagate

### Logging
- Use structured logging (Zap): `logr.Info("msg", zap.String("key", val))`
- Include correlation IDs for request tracing
- Log levels: `debug` (dev), `info` (prod), `warn/error` (always)

---

## 🔒 Security

- **Never commit secrets** — use environment variables
- **Validate all input** — use `validator` tags on DTOs
- **Parameterized queries** — sqlc generates safe queries
- **Dependencies** — `govulncheck` in CI, update deps regularly
- **Rate limiting** — Redis-backed, per-IP and per-user

---

## 📝 Pull Request Checklist

Before requesting review:

- [ ] Branch follows naming convention (`feature/`, `fix/`, etc.)
- [ ] Commit messages follow Conventional Commits
- [ ] All tests pass (`make test-race`)
- [ ] Lint passes (`make lint`)
- [ ] Code formatted (`make fmt`)
- [ ] Tests added/updated for new functionality
- [ ] Documentation updated (README, API docs, comments)
- [ ] No breaking changes (or documented in PR)
- [ ] Database migrations included (if schema changes)
- [ ] CHANGELOG.md updated (for user-facing changes)
- [ ] Self-review completed

---

## 🏷️ Release Process

1. **Prepare release**: `dev` → `main` PR
2. **Squash merge** to `main` (linear history)
3. **Tag release**: `git tag v1.2.3 && git push origin v1.2.3`
4. GitHub Actions:
   - Runs full test suite
   - Builds Docker image
   - Pushes to GHCR: `ghcr.io/PandaX185/clinic-management:v1.2.3`
5. Update CHANGELOG.md

---

## 🤝 Code of Conduct

- Be respectful and constructive
- Focus on code, not people
- Assume good intent
- Ask questions early
- Share knowledge generously

---

## 📞 Getting Help

- **Questions**: Open a Discussion on GitHub
- **Bugs**: Open an Issue with `bug` label
- **Features**: Open an Issue with `enhancement` label
- **Security**: Email security@axiom.example.com (do not open public issue)

---

## 📄 License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).

---

**Thank you for contributing to Axiom!** 🚀