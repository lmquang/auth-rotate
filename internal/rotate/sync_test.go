package rotate

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncOpenAIAndCodexWritesActiveAccount(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "credentials.json")
	openAITargetPath := filepath.Join(tempDir, "auth.json")
	codexTargetPath := filepath.Join(tempDir, "codex.json")

	originalConfig := `{
	  "openai_codex": {
	    "activeEmail": "second@example.com",
	    "accounts": [
	      {
	        "email": "first@example.com",
	        "isActive": true,
	        "openai": {"access": "access-1"},
	        "codex": {"access_token": "token-1"}
	      },
	      {
	        "email": "second@example.com",
	        "isActive": true,
	        "openai": {"access": "access-2"},
	        "codex": {"access_token": "token-2"}
	      }
	    ]
	  }
	}`
	writeFile(t, configPath, originalConfig)
	writeFile(t, openAITargetPath, `{"extra": "keep-me", "openai": {"old": "val"}}`)
	writeFile(t, codexTargetPath, `{"access_token": "old-token"}`)

	var logBuffer bytes.Buffer
	svc := NewService(log.New(&logBuffer, "", 0))

	result, err := svc.SyncOpenAIAndCodex(configPath, openAITargetPath, codexTargetPath)
	if err != nil {
		t.Fatalf("SyncOpenAIAndCodex() error = %v", err)
	}

	if result.PreviousEmail != "second@example.com" {
		t.Fatalf("PreviousEmail = %q, want %q", result.PreviousEmail, "second@example.com")
	}

	if result.SelectedEmail != "second@example.com" {
		t.Fatalf("SelectedEmail = %q, want %q", result.SelectedEmail, "second@example.com")
	}

	if got := readFile(t, configPath); got != originalConfig {
		t.Fatalf("config file changed unexpectedly: got %q want %q", got, originalConfig)
	}

	updatedOpenAI := readFile(t, openAITargetPath)
	assertContains(t, updatedOpenAI, `"access": "access-2"`)
	assertContains(t, updatedOpenAI, `"extra": "keep-me"`)

	updatedCodex := readFile(t, codexTargetPath)
	assertContains(t, updatedCodex, `"access_token": "token-2"`)
	assertNotContains(t, updatedCodex, `old-token`)
}

func TestSyncOpenAIAndCodexErrorsWhenActiveEmailMissing(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "credentials.json")
	openAITargetPath := filepath.Join(tempDir, "auth.json")
	codexTargetPath := filepath.Join(tempDir, "codex.json")

	writeFile(t, configPath, `{
	  "openai_codex": {
	    "activeEmail": "missing@example.com",
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
	writeFile(t, openAITargetPath, `{}`)
	writeFile(t, codexTargetPath, `{}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.SyncOpenAIAndCodex(configPath, openAITargetPath, codexTargetPath)
	if err == nil {
		t.Fatal("expected error")
	}

	if err.Error() != "active openai_codex account not found: missing@example.com" {
		t.Fatalf("error = %q", err)
	}
}

func TestSyncGeminiWritesActiveAccount(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "credentials.json")
	activeCredsPath := filepath.Join(tempDir, "oauth_creds.json")

	originalConfig := `{
	  "gemini": {
	    "activeEmail": "second@gmail.com",
	    "accounts": [
	      {
	        "email": "first@gmail.com",
	        "isActive": true,
	        "data": {"access_token": "token-1"}
	      },
	      {
	        "email": "second@gmail.com",
	        "isActive": true,
	        "data": {"access_token": "token-2"}
	      }
	    ]
	  }
	}`
	writeFile(t, configPath, originalConfig)
	writeFile(t, activeCredsPath, `{"access_token": "old-token"}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	result, err := svc.SyncGemini(configPath, activeCredsPath)
	if err != nil {
		t.Fatalf("SyncGemini() error = %v", err)
	}

	if result.PreviousEmail != "second@gmail.com" {
		t.Fatalf("PreviousEmail = %q, want %q", result.PreviousEmail, "second@gmail.com")
	}

	if result.SelectedEmail != "second@gmail.com" {
		t.Fatalf("SelectedEmail = %q, want %q", result.SelectedEmail, "second@gmail.com")
	}

	if got := readFile(t, configPath); got != originalConfig {
		t.Fatalf("config file changed unexpectedly: got %q want %q", got, originalConfig)
	}

	updatedActive := readFile(t, activeCredsPath)
	assertContains(t, updatedActive, `"access_token": "token-2"`)
	assertNotContains(t, updatedActive, `old-token`)
}

func TestSyncGeminiLeavesConfigUntouchedOnFailure(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "credentials.json")
	activeCredsPath := filepath.Join(tempDir, "oauth_creds.json")

	originalConfig := `{
	  "gemini": {
	    "activeEmail": "first@gmail.com",
	    "accounts": [
	      {
	        "email": "first@gmail.com",
	        "isActive": true,
	        "data": {"access_token": "token-1"}
	      }
	    ]
	  }
	}`
	writeFile(t, configPath, originalConfig)
	writeFile(t, activeCredsPath, `{"access_token": "old-token"}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))
	svc.writeFileAtomic = func(string, []byte, os.FileMode) error {
		return errors.New("boom")
	}

	_, err := svc.SyncGemini(configPath, activeCredsPath)
	if err == nil {
		t.Fatal("expected error")
	}

	if got := readFile(t, configPath); got != originalConfig {
		t.Fatalf("config file changed unexpectedly: got %q want %q", got, originalConfig)
	}
}
