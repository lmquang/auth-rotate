# auth-rotate

Go CLI tool that round-rotates OAuth accounts across providers.

## Providers

### OpenAI

Reads accounts from a source list and updates the active account in a target config.

- **Source**: `~/.local/share/opencode/auth_open_ai.json` — array of account objects
- **Target**: `~/.local/share/opencode/auth.json` — current config with `openai` key

### Gemini

Cycles between the `active` email and the `old` list, then updates the active credentials.

- **Accounts**: `~/.gemini/google_accounts.json` — tracks which email is active
- **Master creds**: `~/.gemini/oauth_cred_gemini.json` — all accounts' credentials (source of truth)
- **Active creds**: `~/.gemini/oauth_creds.json` — credentials for the current active account (what Gemini CLI reads)

```json
{
  "active": "user1@gmail.com",
  "old": ["user2@gmail.com", "user3@gmail.com"]
}
```

After rotation:
1. `active` becomes `user2@gmail.com` and `user1@gmail.com` moves to the end of `old`
2. `oauth_creds.json` is updated with `user2@gmail.com`'s credentials from `oauth_cred_gemini.json`

## Usage

```bash
# Build
make build

# Rotate OpenAI (default)
./bin/auth-rotate

# Rotate OpenAI (explicit)
./bin/auth-rotate -provider openai

# Rotate Gemini
./bin/auth-rotate -provider gemini

# Custom paths
./bin/auth-rotate -source /path/to/auth_open_ai.json -target /path/to/auth.json
./bin/auth-rotate -provider gemini -gemini-path /path/to/google_accounts.json
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-provider` | `openai` | Provider to rotate: `openai` or `gemini` |
| `-source` | `~/.local/share/opencode/auth_open_ai.json` | OpenAI source accounts file |
| `-target` | `~/.local/share/opencode/auth.json` | OpenAI target config file |
| `-gemini-path` | `~/.gemini/google_accounts.json` | Gemini accounts file |

## Features

- **Atomic writes**: writes to a temp file then renames — original file is untouched on failure
- **File locking**: uses `flock` to prevent concurrent rotations
- **Input validation**: checks for missing fields, duplicates, malformed JSON
- **PII masking**: emails are masked in debug logs (e.g., `j***n@example.com`)
- **Gemini creds sync**: updates `oauth_creds.json` with the new active account's credentials

## Development

```bash
make build          # build to bin/auth-rotate
make test           # run all tests
make clean          # remove bin/

# Single test
go test ./internal/rotate/ -run TestRotateNextAccount -v
go test ./internal/rotate/ -run TestRotateGeminiRotatesToNextAccount -v

# Verbose + race detection
go test ./internal/rotate/ -run TestRotateWrapsAround -v -race

# Static analysis
go vet ./...
```

## Project Structure

```
.
├── main.go
├── internal/
│   └── rotate/
│       ├── service.go          # OpenAI rotation, shared types, helpers
│       ├── service_test.go
│       ├── gemini.go            # Gemini rotation
│       └── gemini_test.go
├── Makefile
└── go.mod
```
