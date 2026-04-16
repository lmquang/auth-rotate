package rotate

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotateOpenAIAndCodexToNextActive(t *testing.T) {
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
	}`)

	writeFile(t, openAITargetPath, `{"extra": "keep-me", "openai": {"old": "val"}}`)
	writeFile(t, codexTargetPath, `{"extra_codex": "keep-me", "tokens": {"old": "val"}}`)

	var logBuffer bytes.Buffer
	svc := NewService(log.New(&logBuffer, "", 0))

	result, err := svc.RotateOpenAIAndCodex(configPath, openAITargetPath, codexTargetPath)
	if err != nil {
		t.Fatalf("RotateOpenAIAndCodex() error = %v", err)
	}

	if result.PreviousEmail != "first@example.com" {
		t.Fatalf("PreviousEmail = %q, want %q", result.PreviousEmail, "first@example.com")
	}

	if result.SelectedEmail != "second@example.com" {
		t.Fatalf("SelectedEmail = %q, want %q", result.SelectedEmail, "second@example.com")
	}

	// Verify config was updated
	updatedConfig := readFile(t, configPath)
	assertContains(t, updatedConfig, `"activeEmail": "second@example.com"`)

	// Verify OpenAI target was updated
	updatedOpenAI := readFile(t, openAITargetPath)
	assertContains(t, updatedOpenAI, `"access": "access-2"`)
	assertContains(t, updatedOpenAI, `"extra": "keep-me"`)

	// Verify Codex target was updated
	updatedCodex := readFile(t, codexTargetPath)
	assertContains(t, updatedCodex, `"access_token": "token-2"`)
}

func TestRotateOpenAIAndCodexSkipsInactive(t *testing.T) {
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
	        "openai": {"access": "access-1"},
	        "codex": {"access_token": "token-1"}
	      },
	      {
	        "email": "second@example.com",
	        "isActive": false,
	        "openai": {"access": "access-2"},
	        "codex": {"access_token": "token-2"}
	      },
	      {
	        "email": "third@example.com",
	        "isActive": true,
	        "openai": {"access": "access-3"},
	        "codex": {"access_token": "token-3"}
	      }
	    ]
	  }
	}`)

	writeFile(t, openAITargetPath, `{}`)
	writeFile(t, codexTargetPath, `{}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	result, err := svc.RotateOpenAIAndCodex(configPath, openAITargetPath, codexTargetPath)
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	if result.SelectedEmail != "third@example.com" {
		t.Fatalf("SelectedEmail = %q, want third@example.com", result.SelectedEmail)
	}
}

func TestRotateOpenAIAndCodexLeavesFileUntouchedOnFailure(t *testing.T) {
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
	        "email": "first@example.com",
	        "isActive": true,
	        "openai": {},
	        "codex": {}
	      },
	      {
	        "email": "second@example.com",
	        "isActive": true,
	        "openai": {},
	        "codex": {}
	      }
	    ]
	  }
	}`
	writeFile(t, configPath, originalConfig)
	writeFile(t, openAITargetPath, `{}`)
	writeFile(t, codexTargetPath, `{}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))
	svc.writeFileAtomic = func(string, []byte, os.FileMode) error {
		return errors.New("boom")
	}

	_, err := svc.RotateOpenAIAndCodex(configPath, openAITargetPath, codexTargetPath)
	if err == nil {
		t.Fatal("expected error")
	}

	if got := readFile(t, configPath); got != originalConfig {
		t.Fatal("config file changed unexpectedly")
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	return string(data)
}

func assertContains(t *testing.T, value string, substring string) {
	t.Helper()

	if !strings.Contains(value, substring) {
		t.Fatalf("expected %q to contain %q", value, substring)
	}
}

func assertNotContains(t *testing.T, value string, substring string) {
	t.Helper()

	if strings.Contains(value, substring) {
		t.Fatalf("expected %q not to contain %q", value, substring)
	}
}
