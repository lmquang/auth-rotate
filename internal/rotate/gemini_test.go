package rotate

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func TestRotateGeminiToNextActive(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "credentials.json")
	activeCredsPath := filepath.Join(tempDir, "oauth_creds.json")

	writeFile(t, configPath, `{
	  "gemini": {
	    "activeEmail": "first@gmail.com",
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
	}`)

	writeFile(t, activeCredsPath, `{"old": "val"}`)

	var logBuffer bytes.Buffer
	svc := NewService(log.New(&logBuffer, "", 0))

	result, err := svc.RotateGemini(configPath, activeCredsPath)
	if err != nil {
		t.Fatalf("RotateGemini() error = %v", err)
	}

	if result.PreviousEmail != "first@gmail.com" {
		t.Fatalf("PreviousEmail = %q", result.PreviousEmail)
	}

	if result.SelectedEmail != "second@gmail.com" {
		t.Fatalf("SelectedEmail = %q", result.SelectedEmail)
	}

	updatedConfig := readFile(t, configPath)
	assertContains(t, updatedConfig, `"activeEmail": "second@gmail.com"`)

	updatedActive := readFile(t, activeCredsPath)
	assertContains(t, updatedActive, `"access_token": "token-2"`)
}

func TestRotateGeminiSkipsInactive(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "credentials.json")
	activeCredsPath := filepath.Join(tempDir, "oauth_creds.json")

	writeFile(t, configPath, `{
	  "gemini": {
	    "activeEmail": "first@gmail.com",
	    "accounts": [
	      {
	        "email": "first@gmail.com",
	        "isActive": true,
	        "data": {"access_token": "token-1"}
	      },
	      {
	        "email": "second@gmail.com",
	        "isActive": false,
	        "data": {"access_token": "token-2"}
	      },
	      {
	        "email": "third@gmail.com",
	        "isActive": true,
	        "data": {"access_token": "token-3"}
	      }
	    ]
	  }
	}`)

	writeFile(t, activeCredsPath, `{}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	result, err := svc.RotateGemini(configPath, activeCredsPath)
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	if result.SelectedEmail != "third@gmail.com" {
		t.Fatalf("SelectedEmail = %q, want third@gmail.com", result.SelectedEmail)
	}
}

func TestRotateGeminiLeavesFileUntouchedOnFailure(t *testing.T) {
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
	        "data": {}
	      },
	      {
	        "email": "second@gmail.com",
	        "isActive": true,
	        "data": {}
	      }
	    ]
	  }
	}`
	writeFile(t, configPath, originalConfig)
	writeFile(t, activeCredsPath, `{}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))
	svc.writeFileAtomic = func(string, []byte, os.FileMode) error {
		return errors.New("boom")
	}

	_, err := svc.RotateGemini(configPath, activeCredsPath)
	if err == nil {
		t.Fatal("expected error")
	}

	if got := readFile(t, configPath); got != originalConfig {
		t.Fatal("config file changed unexpectedly")
	}
}
