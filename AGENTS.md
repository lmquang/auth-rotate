# AGENTS.md

Guide for agentic coding agents working in this repository.

## Project Overview

Go CLI tool (`auth-rotate`) that rotates OAuth accounts across providers:

- **OpenAI**: Round-robin rotation through `~/.local/share/opencode/auth_open_ai.json`
  (source list), updating the active account in `~/.local/share/opencode/auth.json`.
- **Gemini**: Round-robin rotation through `~/.gemini/google_accounts.json`,
  cycling between `active` email and `old` list.

Selected via `-provider openai` (default) or `-provider gemini`.

## Build / Lint / Test Commands

```bash
make build          # build binary to bin/auth-rotate
make test           # run all tests: go test ./...
make clean          # remove bin/

# Single test
go test ./internal/rotate/ -run TestRotateNextAccount -v
go test ./internal/rotate/ -run TestRotateGeminiRotatesToNextAccount -v

# Verbose + race detection
go test ./internal/rotate/ -run TestRotateWrapsAround -v -race

# All tests with coverage
go test ./... -cover
```

There is no linter config (`.golangci.yml`). Use `go vet ./...` for static
analysis. Run `go vet ./...` before declaring work complete.

## Code Style Guidelines

### Imports

- Group imports in two blocks: stdlib first, then project imports, separated
  by a blank line. No third-party dependencies exist; keep it that way unless
  there is a strong reason.
- Use the full module path for internal packages:
  `auth-rotate/internal/rotate`

### Formatting

- Run `gofmt -w .` (or rely on editor auto-format). The codebase uses
  standard Go formatting — tabs for indentation, no trailing whitespace.

### Types & Naming

- Exported types use PascalCase (`Service`, `Result`, `GeminiResult`,
  `GeminiAccounts`, `OpenAIAuth`, `AuthConfig`, `Account`).
- Unexported functions and constants use camelCase (`selectNextAccount`,
  `validateAccount`, `writeFileAtomic`, `lockFile`, `maskEmail`,
  `openAIProviderKey`).
- Receiver name is `s` for `*Service`.
- JSON struct tags use the `json:"fieldName"` convention matching upstream
  API keys (e.g., `accountId`).

### Error Handling

- Wrap errors with context using `fmt.Errorf("operation: %w", err)`.
- Return typed errors with `errors.New("message")` for validation failures.
- Never ignore returned errors. Discard explicitly only for cleanup in
  `defer` when the operation is best-effort (use `_ = expr`).
- Guard clauses over nested if-else. Return early on errors.

### Logging

- Use `log.Logger` passed via dependency injection; never use `log` package
  globals directly.
- Debug logs use the helper method `(s *Service).debug(format, args...)`
  which prepends `DEBUG ` to every line.
- Log format: `DEBUG <scope> <key>=<value>` pairs — structured, parseable.
- Mask sensitive data in logs (emails via `maskEmail`, never log tokens).

### Architecture

- Service struct holds dependencies (`logger`, `writeFileAtomic`) for
  testability. Inject behaviors via struct fields (not interfaces) for this
  small codebase.
- Atomic file writes: write to a temp file in the same directory, `chmod`,
  then `os.Rename`. Original file is untouched on failure.
- File locking via `syscall.Flock` with `.lock` suffix files.

### Testing Conventions

- Test files are package-internal (`package rotate`, not `package rotate_test`).
- All tests call `t.Parallel()` as the first statement.
- Use `t.TempDir()` for filesystem fixtures; never write to real paths.
- Use `t.Helper()` in test utility functions (`writeFile`, `readFile`,
  `assertContains`, `assertNotContains`).
- Prefer `t.Fatalf` for assertion failures with descriptive messages showing
  `got` and `want`.
- Test names follow `TestRotate<Behavior>` pattern (e.g.,
  `TestRotateWrapsAround`, `TestRotateErrorsForMalformedJSON`).
  Gemini tests use `TestRotateGemini<Behavior>` (e.g.,
  `TestRotateGeminiRotatesToNextAccount`,
  `TestRotateGeminiLeavesOriginalFileUntouchedOnWriteFailure`).
- Inject mock behaviors by replacing struct fields (e.g.,
  `svc.writeFileAtomic = func(...) error { ... }`).
- Verify side effects by reading files back; verify logs are captured in
  `bytes.Buffer`.
- Verify sensitive data does NOT leak into logs (`assertNotContains`).

### General Principles

- Keep the dependency list at zero. This is a deliberate, self-contained
  tool — no frameworks.
- Prefer stdlib solutions. The codebase uses `encoding/json`, `os`,
  `path/filepath`, `syscall`, `strings`, `log`, `flag`.
- When adding new packages under `internal/`, follow the same conventions:
  package-internal tests, `*Service` receiver pattern, dependency injection.
