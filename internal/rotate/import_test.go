package rotate

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func TestImportOpenCodeUpdatesMatchingAccount(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "credentials.json")
	openAITargetPath := filepath.Join(tempDir, "auth.json")
	codexTargetPath := filepath.Join(tempDir, "codex.json")

	writeFile(t, configPath, `{
	  "openai_codex": {
	    "activeEmail": "first@example.com",
	    "accounts": [
	      {
	        "accountId": "acct-1",
	        "email": "first@example.com",
	        "isActive": true,
	        "openai": {"access": "old-access", "accountId": "acct-1"},
	        "codex": {"tokens": {"account_id": "acct-1", "access_token": "token-1"}}
	      },
	      {
	        "accountId": "acct-2",
	        "email": "second@example.com",
	        "isActive": true,
	        "openai": {"access": "stale-access", "accountId": "acct-2"},
	        "codex": {"auth_mode": "chatgpt", "tokens": {"account_id": "acct-2", "access_token": "token-2", "id_token": "keep-me"}}
	      }
	    ]
	  }
	}`)
	writeFile(t, openAITargetPath, `{
	  "openai": {
	    "type": "oauth",
	    "access": "new-access",
	    "refresh": "new-refresh",
	    "expires": 123,
	    "accountId": "acct-2"
	  },
	  "openrouter": {
	    "type": "api",
	    "key": "keep-out-of-central"
	  }
	}`)
	writeFile(t, codexTargetPath, `{
	  "auth_mode": "chatgpt",
	  "tokens": {
	    "account_id": "acct-2",
	    "access_token": "old-token",
	    "id_token": "keep-me"
	  }
	}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	result, err := svc.ImportOpenCode(configPath, openAITargetPath, codexTargetPath)
	if err != nil {
		t.Fatalf("ImportOpenCode() error = %v", err)
	}

	if result.PreviousEmail != "first@example.com" {
		t.Fatalf("PreviousEmail = %q, want %q", result.PreviousEmail, "first@example.com")
	}

	if result.SelectedEmail != "second@example.com" {
		t.Fatalf("SelectedEmail = %q, want %q", result.SelectedEmail, "second@example.com")
	}

	updatedConfig := readFile(t, configPath)
	assertContains(t, updatedConfig, `"activeEmail": "second@example.com"`)
	assertContains(t, updatedConfig, `"accountId": "acct-2"`)
	assertContains(t, updatedConfig, `"access": "new-access"`)
	assertContains(t, updatedConfig, `"refresh": "new-refresh"`)
	assertContains(t, updatedConfig, `"access_token": "new-access"`)
	assertContains(t, updatedConfig, `"refresh_token": "new-refresh"`)
	assertContains(t, updatedConfig, `"account_id": "acct-2"`)
	assertContains(t, updatedConfig, `"id_token": "keep-me"`)
	assertNotContains(t, updatedConfig, `keep-out-of-central`)

	updatedCodex := readFile(t, codexTargetPath)
	assertContains(t, updatedCodex, `"access_token": "new-access"`)
	assertContains(t, updatedCodex, `"refresh_token": "new-refresh"`)
	assertContains(t, updatedCodex, `"account_id": "acct-2"`)
	assertContains(t, updatedCodex, `"id_token": "keep-me"`)
}

func jwtWithExp(t *testing.T, exp int64) string {
	t.Helper()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, err := json.Marshal(map[string]int64{"exp": exp})
	if err != nil {
		t.Fatalf("encode jwt payload: %v", err)
	}

	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func TestImportOpenCodeCreatesCodexTokensWhenMissing(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "credentials.json")
	openAITargetPath := filepath.Join(tempDir, "auth.json")
	codexTargetPath := filepath.Join(tempDir, "codex.json")

	writeFile(t, configPath, `{
	  "openai_codex": {
	    "activeEmail": "first@example.com",
	    "accounts": [
	      {
	        "accountId": "acct-2",
	        "email": "second@example.com",
	        "isActive": true,
	        "openai": {"access": "stale-access", "accountId": "acct-2"},
	        "codex": {}
	      }
	    ]
	  }
	}`)
	writeFile(t, openAITargetPath, `{
	  "openai": {
	    "type": "oauth",
	    "access": "new-access",
	    "refresh": "new-refresh",
	    "expires": 123,
	    "accountId": "acct-2"
	  }
	}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.ImportOpenCode(configPath, openAITargetPath, codexTargetPath)
	if err != nil {
		t.Fatalf("ImportOpenCode() error = %v", err)
	}

	updatedConfig := readFile(t, configPath)
	assertContains(t, updatedConfig, `"access_token": "new-access"`)
	assertContains(t, updatedConfig, `"refresh_token": "new-refresh"`)
	assertContains(t, updatedConfig, `"account_id": "acct-2"`)

	updatedCodex := readFile(t, codexTargetPath)
	assertContains(t, updatedCodex, `"access_token": "new-access"`)
	assertContains(t, updatedCodex, `"refresh_token": "new-refresh"`)
	assertContains(t, updatedCodex, `"account_id": "acct-2"`)
}

func TestImportCodexMatchesExistingRawAccountID(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "credentials.json")
	openAITargetPath := filepath.Join(tempDir, "auth.json")
	codexTargetPath := filepath.Join(tempDir, "codex.json")

	writeFile(t, configPath, `{
	  "openai_codex": {
	    "activeEmail": "first@example.com",
	    "accounts": [
	      {
	        "email": "first@example.com",
	        "isActive": true,
	        "openai": {"access": "access-1", "accountId": "acct-1"},
	        "codex": {"tokens": {"account_id": "acct-1", "access_token": "token-1"}}
	      },
	      {
	        "email": "second@example.com",
	        "isActive": true,
	        "openai": {"access": "access-2", "refresh": "refresh-2", "accountId": "acct-2", "expires": 123, "extra": "keep-me"},
	        "codex": {"tokens": {"account_id": "acct-2", "access_token": "token-2"}}
	      }
	    ]
	  }
	}`)
	accessToken := jwtWithExp(t, 1893456000)
	writeFile(t, openAITargetPath, `{
	  "openai": {
	    "type": "oauth",
	    "access": "stale-access",
	    "refresh": "stale-refresh",
	    "accountId": "old-id"
	  },
	  "openrouter": {
	    "type": "api",
	    "key": "keep-me"
	  }
	}`)
	writeFile(t, codexTargetPath, `{
	  "OPENAI_API_KEY": null,
	  "auth_mode": "chatgpt",
	  "last_refresh": "2026-04-25T05:19:48.220040Z",
	  "tokens": {
	    "access_token": "`+accessToken+`",
	    "account_id": "acct-2",
	    "id_token": "fresh-id-token",
	    "refresh_token": "fresh-refresh"
	  }
	}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	result, err := svc.ImportCodex(configPath, openAITargetPath, codexTargetPath)
	if err != nil {
		t.Fatalf("ImportCodex() error = %v", err)
	}

	if result.PreviousEmail != "first@example.com" {
		t.Fatalf("PreviousEmail = %q, want %q", result.PreviousEmail, "first@example.com")
	}

	if result.SelectedEmail != "second@example.com" {
		t.Fatalf("SelectedEmail = %q, want %q", result.SelectedEmail, "second@example.com")
	}

	updatedConfig := readFile(t, configPath)
	assertContains(t, updatedConfig, `"activeEmail": "second@example.com"`)
	assertContains(t, updatedConfig, `"accountId": "acct-2"`)
	assertContains(t, updatedConfig, `"access": "`+accessToken+`"`)
	assertContains(t, updatedConfig, `"refresh": "fresh-refresh"`)
	assertContains(t, updatedConfig, `"expires": 1893456000000`)
	assertContains(t, updatedConfig, `"extra": "keep-me"`)
	assertContains(t, updatedConfig, `"access_token": "`+accessToken+`"`)
	assertContains(t, updatedConfig, `"id_token": "fresh-id-token"`)

	updatedOpenAI := readFile(t, openAITargetPath)
	assertContains(t, updatedOpenAI, `"access": "`+accessToken+`"`)
	assertContains(t, updatedOpenAI, `"refresh": "fresh-refresh"`)
	assertContains(t, updatedOpenAI, `"accountId": "acct-2"`)
	assertContains(t, updatedOpenAI, `"expires": 1893456000000`)
	assertContains(t, updatedOpenAI, `"extra": "keep-me"`)
	assertContains(t, updatedOpenAI, `"key": "keep-me"`)

	var target struct {
		OpenAI map[string]any `json:"openai"`
	}
	if err := json.Unmarshal([]byte(updatedOpenAI), &target); err != nil {
		t.Fatalf("decode updated openai target: %v", err)
	}

	expiresValue, ok := target.OpenAI["expires"].(float64)
	if !ok {
		t.Fatalf("expires type = %T, want float64", target.OpenAI["expires"])
	}

	if expiresValue != 1893456000000 {
		t.Fatalf("expires = %.0f, want %d", expiresValue, int64(1893456000000))
	}
}

func TestImportCodexAddsNewAccountWhenMissing(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "credentials.json")
	openAITargetPath := filepath.Join(tempDir, "auth.json")
	codexTargetPath := filepath.Join(tempDir, "codex.json")

	writeFile(t, configPath, `{
	  "openai_codex": {
	    "activeEmail": "first@example.com",
	    "accounts": [
	      {
	        "accountId": "acct-1",
	        "email": "first@example.com",
	        "isActive": true,
	        "openai": {"access": "access-1", "accountId": "acct-1"},
	        "codex": {"tokens": {"account_id": "acct-1", "access_token": "token-1"}}
	      }
	    ]
	  }
	}`)
	writeFile(t, codexTargetPath, `{
	  "email": "second@example.com",
	  "auth_mode": "chatgpt",
	  "tokens": {
	    "account_id": "acct-2",
	    "access_token": "fresh-token",
	    "refresh_token": "fresh-refresh"
	  }
	}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	result, err := svc.ImportCodex(configPath, openAITargetPath, codexTargetPath)
	if err != nil {
		t.Fatalf("ImportCodex() error = %v", err)
	}

	if result.PreviousEmail != "first@example.com" {
		t.Fatalf("PreviousEmail = %q, want %q", result.PreviousEmail, "first@example.com")
	}

	if result.SelectedEmail != "second@example.com" {
		t.Fatalf("SelectedEmail = %q, want %q", result.SelectedEmail, "second@example.com")
	}

	if result.AccountCount != 2 {
		t.Fatalf("AccountCount = %d, want %d", result.AccountCount, 2)
	}

	updatedConfig := readFile(t, configPath)
	assertContains(t, updatedConfig, `"activeEmail": "second@example.com"`)
	assertContains(t, updatedConfig, `"accountId": "acct-2"`)
	assertContains(t, updatedConfig, `"email": "second@example.com"`)
	assertContains(t, updatedConfig, `"isActive": true`)
	assertContains(t, updatedConfig, `"access": "fresh-token"`)
	assertContains(t, updatedConfig, `"refresh": "fresh-refresh"`)
	assertContains(t, updatedConfig, `"type": "oauth"`)
	assertContains(t, updatedConfig, `"access_token": "fresh-token"`)
	assertContains(t, updatedConfig, `"refresh_token": "fresh-refresh"`)

	updatedOpenAI := readFile(t, openAITargetPath)
	assertContains(t, updatedOpenAI, `"accountId": "acct-2"`)
	assertContains(t, updatedOpenAI, `"access": "fresh-token"`)
	assertContains(t, updatedOpenAI, `"refresh": "fresh-refresh"`)
}

func TestImportCodexPromptsForNewAccountEmailWhenMissing(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "credentials.json")
	openAITargetPath := filepath.Join(tempDir, "auth.json")
	codexTargetPath := filepath.Join(tempDir, "codex.json")

	writeFile(t, configPath, `{
	  "openai_codex": {
	    "activeEmail": "first@example.com",
	    "accounts": [
	      {
	        "accountId": "acct-1",
	        "email": "first@example.com",
	        "isActive": true,
	        "openai": {"access": "access-1", "accountId": "acct-1"},
	        "codex": {"tokens": {"account_id": "acct-1", "access_token": "token-1"}}
	      }
	    ]
	  }
	}`)
	writeFile(t, codexTargetPath, `{
	  "auth_mode": "chatgpt",
	  "tokens": {
	    "account_id": "acct-2",
	    "access_token": "fresh-token",
	    "refresh_token": "fresh-refresh"
	  }
	}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))
	svc.promptInput = func(prompt string) (string, error) {
		if prompt != "Enter email for new Codex account acct-2: " {
			t.Fatalf("prompt = %q, want %q", prompt, "Enter email for new Codex account acct-2: ")
		}
		return "second@example.com\n", nil
	}

	result, err := svc.ImportCodex(configPath, openAITargetPath, codexTargetPath)
	if err != nil {
		t.Fatalf("ImportCodex() error = %v", err)
	}

	if result.SelectedEmail != "second@example.com" {
		t.Fatalf("SelectedEmail = %q, want %q", result.SelectedEmail, "second@example.com")
	}

	updatedConfig := readFile(t, configPath)
	assertContains(t, updatedConfig, `"activeEmail": "second@example.com"`)
	assertContains(t, updatedConfig, `"email": "second@example.com"`)
	assertContains(t, updatedConfig, `"accountId": "acct-2"`)
}

func TestImportCodexErrorsWhenAccountIDMissing(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "credentials.json")
	openAITargetPath := filepath.Join(tempDir, "auth.json")
	codexTargetPath := filepath.Join(tempDir, "codex.json")

	writeFile(t, configPath, `{
	  "openai_codex": {
	    "activeEmail": "first@example.com",
	    "accounts": [
	      {
	        "email": "first@example.com",
	        "isActive": true,
	        "openai": {},
	        "codex": {}
	      }
	    ]
	  }
	}`)
	writeFile(t, codexTargetPath, `{
	  "tokens": {
	    "access_token": "fresh-token"
	  }
	}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.ImportCodex(configPath, openAITargetPath, codexTargetPath)
	if err == nil {
		t.Fatal("expected error")
	}

	if err.Error() != "codex target missing tokens.account_id" {
		t.Fatalf("error = %q", err)
	}
}

func TestImportOpenCodeLeavesConfigUntouchedOnFailure(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "credentials.json")
	openAITargetPath := filepath.Join(tempDir, "auth.json")
	codexTargetPath := filepath.Join(tempDir, "codex.json")

	originalConfig := `{
	  "openai_codex": {
	    "activeEmail": "first@example.com",
	    "accounts": [
	      {
	        "accountId": "acct-1",
	        "email": "first@example.com",
	        "isActive": true,
	        "openai": {"access": "old-access", "accountId": "acct-1"},
	        "codex": {"tokens": {"account_id": "acct-1", "access_token": "token-1"}}
	      }
	    ]
	  }
	}`
	writeFile(t, configPath, originalConfig)
	writeFile(t, openAITargetPath, `{
	  "openai": {
	    "type": "oauth",
	    "access": "new-access",
	    "refresh": "new-refresh",
	    "expires": 123,
	    "accountId": "acct-1"
	  }
	}`)
	originalCodexTarget := `{
	  "auth_mode": "chatgpt",
	  "tokens": {
	    "account_id": "acct-1",
	    "access_token": "token-1"
	  }
	}`
	writeFile(t, codexTargetPath, originalCodexTarget)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))
	svc.writeFileAtomic = func(string, []byte, os.FileMode) error {
		return errors.New("boom")
	}

	_, err := svc.ImportOpenCode(configPath, openAITargetPath, codexTargetPath)
	if err == nil {
		t.Fatal("expected error")
	}

	if got := readFile(t, configPath); got != originalConfig {
		t.Fatalf("config file changed unexpectedly: got %q want %q", got, originalConfig)
	}

	if got := readFile(t, codexTargetPath); got != originalCodexTarget {
		t.Fatalf("codex target changed unexpectedly: got %q want %q", got, originalCodexTarget)
	}
}
