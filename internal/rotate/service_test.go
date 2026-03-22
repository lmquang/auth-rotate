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

func TestRotateNextAccount(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "auth_open_ai.json")
	targetPath := filepath.Join(tempDir, "auth.json")

	writeFile(t, sourcePath, `[
	  {
	    "email": "first@example.com",
	    "openai": {
	      "type": "oauth",
	      "access": "access-1",
	      "refresh": "refresh-1",
	      "expires": 111,
	      "accountId": "account-1"
	    }
	  },
	  {
	    "email": "second@example.com",
	    "openai": {
	      "type": "oauth",
	      "access": "access-2",
	      "refresh": "refresh-2",
	      "expires": 222,
	      "accountId": "account-2"
	    }
	  }
	]`)

	writeFile(t, targetPath, `{
	  "openrouter": {
	    "type": "api",
	    "key": "keep-me"
	  },
	  "openai": {
	    "type": "oauth",
	    "access": "access-1",
	    "refresh": "refresh-1",
	    "expires": 111,
	    "accountId": "account-1"
	  },
	  "extra": {
	    "enabled": true
	  }
	}`)

	var logBuffer bytes.Buffer
	svc := NewService(log.New(&logBuffer, "", 0))

	result, err := svc.Rotate(sourcePath, targetPath)
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}

	if result.PreviousAccountID != "account-1" {
		t.Fatalf("PreviousAccountID = %q, want %q", result.PreviousAccountID, "account-1")
	}

	if result.SelectedAccountID != "account-2" {
		t.Fatalf("SelectedAccountID = %q, want %q", result.SelectedAccountID, "account-2")
	}

	updated := readFile(t, targetPath)
	assertContains(t, updated, `"accountId": "account-2"`)
	assertContains(t, updated, `"access": "access-2"`)
	assertContains(t, updated, `"refresh": "refresh-2"`)
	assertContains(t, updated, `"key": "keep-me"`)
	assertContains(t, updated, `"enabled": true`)

	logs := logBuffer.String()
	assertContains(t, logs, "DEBUG rotate start")
	assertContains(t, logs, "selected_account_id=account-2")
	assertNotContains(t, logs, "access-2")
	assertNotContains(t, logs, "refresh-2")
	assertNotContains(t, logs, "second@example.com")
	assertContains(t, logs, "s***d@example.com")
}

func TestRotateWrapsAround(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "auth_open_ai.json")
	targetPath := filepath.Join(tempDir, "auth.json")

	writeFile(t, sourcePath, `[
	  {
	    "email": "first@example.com",
	    "openai": {
	      "type": "oauth",
	      "access": "access-1",
	      "refresh": "refresh-1",
	      "expires": 111,
	      "accountId": "account-1"
	    }
	  },
	  {
	    "email": "second@example.com",
	    "openai": {
	      "type": "oauth",
	      "access": "access-2",
	      "refresh": "refresh-2",
	      "expires": 222,
	      "accountId": "account-2"
	    }
	  }
	]`)

	writeFile(t, targetPath, `{
	  "openai": {
	    "type": "oauth",
	    "access": "access-2",
	    "refresh": "refresh-2",
	    "expires": 222,
	    "accountId": "account-2"
	  }
	}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	result, err := svc.Rotate(sourcePath, targetPath)
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}

	if result.SelectedAccountID != "account-1" {
		t.Fatalf("SelectedAccountID = %q, want %q", result.SelectedAccountID, "account-1")
	}

	updated := readFile(t, targetPath)
	assertContains(t, updated, `"accountId": "account-1"`)
	assertContains(t, updated, `"access": "access-1"`)
	assertContains(t, updated, `"refresh": "refresh-1"`)
}

func TestRotateSingleAccountStaysStable(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "auth_open_ai.json")
	targetPath := filepath.Join(tempDir, "auth.json")

	writeFile(t, sourcePath, `[
	  {
	    "email": "solo@example.com",
	    "openai": {
	      "type": "oauth",
	      "access": "access-1",
	      "refresh": "refresh-1",
	      "expires": 111,
	      "accountId": "account-1"
	    }
	  }
	]`)

	writeFile(t, targetPath, `{
	  "openai": {
	    "type": "oauth",
	    "access": "old-access",
	    "refresh": "old-refresh",
	    "expires": 100,
	    "accountId": "account-1"
	  }
	}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	result, err := svc.Rotate(sourcePath, targetPath)
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}

	if result.SelectedAccountID != "account-1" {
		t.Fatalf("SelectedAccountID = %q, want %q", result.SelectedAccountID, "account-1")
	}
}

func TestRotateErrorsWhenCurrentAccountMissing(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "auth_open_ai.json")
	targetPath := filepath.Join(tempDir, "auth.json")

	writeFile(t, sourcePath, `[
	  {
	    "email": "first@example.com",
	    "openai": {
	      "type": "oauth",
	      "access": "access-1",
	      "refresh": "refresh-1",
	      "expires": 111,
	      "accountId": "account-1"
	    }
	  }
	]`)

	original := `{
	  "openai": {
	    "type": "oauth",
	    "access": "old-access",
	    "refresh": "old-refresh",
	    "expires": 100,
	    "accountId": "missing-account"
	  }
	}`
	writeFile(t, targetPath, original)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.Rotate(sourcePath, targetPath)
	if err == nil {
		t.Fatal("Rotate() error = nil, want error")
	}

	assertContains(t, err.Error(), "current accountId")
	if got := readFile(t, targetPath); got != original {
		t.Fatalf("target file changed unexpectedly\n got: %s\nwant: %s", got, original)
	}
}

func TestRotateErrorsForEmptySourceList(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "auth_open_ai.json")
	targetPath := filepath.Join(tempDir, "auth.json")

	writeFile(t, sourcePath, `[]`)
	writeFile(t, targetPath, `{"openai":{"accountId":"account-1"}}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.Rotate(sourcePath, targetPath)
	if err == nil {
		t.Fatal("Rotate() error = nil, want error")
	}

	assertContains(t, err.Error(), "no accounts")
}

func TestRotateErrorsForDuplicateAccountIDs(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "auth_open_ai.json")
	targetPath := filepath.Join(tempDir, "auth.json")

	writeFile(t, sourcePath, `[
	  {
	    "email": "first@example.com",
	    "openai": {
	      "type": "oauth",
	      "access": "access-1",
	      "refresh": "refresh-1",
	      "expires": 111,
	      "accountId": "account-1"
	    }
	  },
	  {
	    "email": "second@example.com",
	    "openai": {
	      "type": "oauth",
	      "access": "access-2",
	      "refresh": "refresh-2",
	      "expires": 222,
	      "accountId": "account-1"
	    }
	  }
	]`)
	writeFile(t, targetPath, `{"openai":{"accountId":"account-1"}}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.Rotate(sourcePath, targetPath)
	if err == nil {
		t.Fatal("Rotate() error = nil, want error")
	}

	assertContains(t, err.Error(), "duplicate openai.accountId")
}

func TestRotateErrorsForMalformedJSON(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "auth_open_ai.json")
	targetPath := filepath.Join(tempDir, "auth.json")

	writeFile(t, sourcePath, `{not-json}`)
	writeFile(t, targetPath, `{"openai":{"accountId":"account-1"}}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.Rotate(sourcePath, targetPath)
	if err == nil {
		t.Fatal("Rotate() error = nil, want error")
	}

	assertContains(t, err.Error(), "decode source accounts")
}

func TestRotateErrorsForMissingOpenAIConfig(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "auth_open_ai.json")
	targetPath := filepath.Join(tempDir, "auth.json")

	writeFile(t, sourcePath, `[
	  {
	    "email": "first@example.com",
	    "openai": {
	      "type": "oauth",
	      "access": "access-1",
	      "refresh": "refresh-1",
	      "expires": 111,
	      "accountId": "account-1"
	    }
	  }
	]`)
	writeFile(t, targetPath, `{"openrouter":{"type":"api","key":"keep-me"}}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.Rotate(sourcePath, targetPath)
	if err == nil {
		t.Fatal("Rotate() error = nil, want error")
	}

	assertContains(t, err.Error(), "missing openai")
}

func TestRotateLeavesOriginalFileUntouchedOnWriteFailure(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "auth_open_ai.json")
	targetPath := filepath.Join(tempDir, "auth.json")

	writeFile(t, sourcePath, `[
	  {
	    "email": "first@example.com",
	    "openai": {
	      "type": "oauth",
	      "access": "access-1",
	      "refresh": "refresh-1",
	      "expires": 111,
	      "accountId": "account-1"
	    }
	  },
	  {
	    "email": "second@example.com",
	    "openai": {
	      "type": "oauth",
	      "access": "access-2",
	      "refresh": "refresh-2",
	      "expires": 222,
	      "accountId": "account-2"
	    }
	  }
	]`)

	original := `{
	  "openai": {
	    "type": "oauth",
	    "access": "access-1",
	    "refresh": "refresh-1",
	    "expires": 111,
	    "accountId": "account-1"
	  }
	}`
	writeFile(t, targetPath, original)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))
	svc.writeFileAtomic = func(string, []byte, os.FileMode) error {
		return errors.New("boom")
	}

	_, err := svc.Rotate(sourcePath, targetPath)
	if err == nil {
		t.Fatal("Rotate() error = nil, want error")
	}

	assertContains(t, err.Error(), "write auth target")
	if got := readFile(t, targetPath); got != original {
		t.Fatalf("target file changed unexpectedly\n got: %s\nwant: %s", got, original)
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
