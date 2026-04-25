# auth-rotate

Go CLI tool that rotates, syncs, and imports OAuth accounts from a central credentials file.

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

# Rotate OpenAI/Codex (default)
./bin/auth-rotate

# Rotate OpenAI/Codex (explicit)
./bin/auth-rotate rotate -provider openai

# Rotate Gemini
./bin/auth-rotate rotate -provider gemini

# Sync current OpenAI/Codex account from credentials.json
./bin/auth-rotate sync

# Sync current Gemini account from credentials.json
./bin/auth-rotate sync -provider gemini

# Import current OpenCode auth.json into credentials.json
./bin/auth-rotate import -provider opencode

# Import current Codex auth.json into credentials.json
./bin/auth-rotate import -provider codex

# Custom paths
./bin/auth-rotate sync -config /path/to/credentials.json -openai-target /path/to/auth.json -codex-target /path/to/codex-auth.json
./bin/auth-rotate rotate -provider gemini -gemini-target /path/to/oauth_creds.json
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-provider` | `openai` | Provider to use: `openai` or `gemini` |
| `-config` | `~/.config/auth-rotate/credentials.json` | Central credentials file |
| `-openai-target` | `~/.local/share/opencode/auth.json` | OpenCode target config file |
| `-codex-target` | `~/.codex/auth.json` | Codex target config file |
| `-gemini-target` | `~/.gemini/oauth_creds.json` | Gemini active credentials file |

## Commands

- `rotate`: move to the next active account and write it to the provider target files
- `sync`: re-apply the currently selected account from `credentials.json` to the provider target files without rotating
- `import`: read the current tool auth file and merge it back into `credentials.json`

OpenAI/Codex import matches accounts using a normalized central `accountId` field:

- OpenCode source uses `openai.accountId`
- Codex source uses `tokens.account_id`
- Central credentials store the normalized value as `accountId`

## Features

- **Atomic writes**: writes to a temp file then renames — original file is untouched on failure
- **File locking**: uses `flock` to prevent concurrent rotations
- **Input validation**: checks for missing fields, duplicates, malformed JSON
- **PII masking**: emails are masked in debug logs (e.g., `j***n@example.com`)
- **Gemini creds sync**: updates `oauth_creds.json` with the selected account's credentials

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
│       ├── service.go          # OpenAI/Codex rotation, sync, import, shared helpers
│       ├── service_test.go
│       ├── gemini.go           # Gemini rotation and sync
│       ├── gemini_test.go
│       ├── import_test.go
│       └── sync_test.go
├── Makefile
└── go.mod
```
