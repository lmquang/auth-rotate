package rotate

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func writeMasterCreds(t *testing.T, path string, entries []GeminiCredEntry) {
	t.Helper()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		t.Fatalf("marshal master creds: %v", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write master creds: %v", err)
	}
}

func writeActiveCreds(t *testing.T, path string, data string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatalf("write active creds: %v", err)
	}
}

func defaultCredsSetup(t *testing.T, tempDir string) (masterPath, activePath string) {
	t.Helper()

	masterPath = filepath.Join(tempDir, "oauth_cred_gemini.json")
	activePath = filepath.Join(tempDir, "oauth_creds.json")

	entries := []GeminiCredEntry{
		{Email: "user1@example.com", Data: json.RawMessage(`{"access_token": "token1", "email": "user1@example.com"}`)},
		{Email: "user2@example.com", Data: json.RawMessage(`{"access_token": "token2", "email": "user2@example.com"}`)},
		{Email: "user3@example.com", Data: json.RawMessage(`{"access_token": "token3", "email": "user3@example.com"}`)},
	}
	writeMasterCreds(t, masterPath, entries)
	writeActiveCreds(t, activePath, `{"access_token": "token1"}`)

	return masterPath, activePath
}

func TestRotateGeminiRotatesToNextAccount(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "google_accounts.json")
	masterPath, activePath := defaultCredsSetup(t, tempDir)

	writeFile(t, targetPath, `{
  "active": "user1@example.com",
  "old": [
    "user3@example.com",
    "user4@example.com",
    "user2@example.com"
  ]
}`)

	var logBuffer bytes.Buffer
	svc := NewService(log.New(&logBuffer, "", 0))

	result, err := svc.RotateGemini(targetPath, masterPath, activePath)
	if err != nil {
		t.Fatalf("RotateGemini() error = %v", err)
	}

	if result.PreviousEmail != "user1@example.com" {
		t.Fatalf("PreviousEmail = %q, want %q", result.PreviousEmail, "user1@example.com")
	}

	if result.SelectedEmail != "user3@example.com" {
		t.Fatalf("SelectedEmail = %q, want %q", result.SelectedEmail, "user3@example.com")
	}

	if result.AccountCount != 4 {
		t.Fatalf("AccountCount = %d, want %d", result.AccountCount, 4)
	}

	updated := readFile(t, targetPath)

	var accounts GeminiAccounts
	if err := json.Unmarshal([]byte(updated), &accounts); err != nil {
		t.Fatalf("decode updated file: %v", err)
	}

	if accounts.Active != "user3@example.com" {
		t.Fatalf("active = %q, want %q", accounts.Active, "user3@example.com")
	}

	assertContains(t, updated, "user1@example.com")
	assertContains(t, updated, "user4@example.com")
	assertContains(t, updated, "user2@example.com")

	logs := logBuffer.String()
	assertContains(t, logs, "DEBUG rotate_gemini start")
	assertContains(t, logs, "rotate_gemini selected")
	assertContains(t, logs, "selected_email=u***3@example.com")
	assertNotContains(t, logs, "user3@example.com")
	assertContains(t, logs, "u***1@example.com")

	activeCreds := readFile(t, activePath)
	assertContains(t, activeCreds, "token3")
	assertContains(t, activeCreds, "user3@example.com")
}

func TestRotateGeminiWrapsAround(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "google_accounts.json")
	masterPath, activePath := defaultCredsSetup(t, tempDir)

	writeFile(t, targetPath, `{
  "active": "first@example.com",
  "old": ["second@example.com"]
}`)

	entries := []GeminiCredEntry{
		{Email: "first@example.com", Data: json.RawMessage(`{"access_token": "first_token"}`)},
		{Email: "second@example.com", Data: json.RawMessage(`{"access_token": "second_token"}`)},
	}
	writeMasterCreds(t, masterPath, entries)
	writeActiveCreds(t, activePath, `{"access_token": "first_token"}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	result, err := svc.RotateGemini(targetPath, masterPath, activePath)
	if err != nil {
		t.Fatalf("RotateGemini() error = %v", err)
	}

	if result.SelectedEmail != "second@example.com" {
		t.Fatalf("SelectedEmail = %q, want %q", result.SelectedEmail, "second@example.com")
	}

	activeCreds := readFile(t, activePath)
	assertContains(t, activeCreds, "second_token")

	result, err = svc.RotateGemini(targetPath, masterPath, activePath)
	if err != nil {
		t.Fatalf("RotateGemini() error = %v", err)
	}

	if result.SelectedEmail != "first@example.com" {
		t.Fatalf("SelectedEmail = %q, want %q", result.SelectedEmail, "first@example.com")
	}

	activeCreds = readFile(t, activePath)
	assertContains(t, activeCreds, "first_token")
}

func TestRotateGeminiSingleAccountStaysStable(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "google_accounts.json")
	masterPath, activePath := defaultCredsSetup(t, tempDir)

	writeFile(t, targetPath, `{
  "active": "solo@example.com",
  "old": []
}`)

	entries := []GeminiCredEntry{
		{Email: "solo@example.com", Data: json.RawMessage(`{"access_token": "solo_token"}`)},
	}
	writeMasterCreds(t, masterPath, entries)
	writeActiveCreds(t, activePath, `{"access_token": "old_token"}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	result, err := svc.RotateGemini(targetPath, masterPath, activePath)
	if err != nil {
		t.Fatalf("RotateGemini() error = %v", err)
	}

	if result.SelectedEmail != "solo@example.com" {
		t.Fatalf("SelectedEmail = %q, want %q", result.SelectedEmail, "solo@example.com")
	}

	updated := readFile(t, targetPath)

	var accounts GeminiAccounts
	if err := json.Unmarshal([]byte(updated), &accounts); err != nil {
		t.Fatalf("decode updated file: %v", err)
	}

	if accounts.Active != "solo@example.com" {
		t.Fatalf("active = %q, want %q", accounts.Active, "solo@example.com")
	}

	if len(accounts.Old) != 0 {
		t.Fatalf("old = %v, want empty", accounts.Old)
	}

	activeCreds := readFile(t, activePath)
	assertContains(t, activeCreds, "solo_token")
}

func TestRotateGeminiErrorsForMalformedJSON(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "google_accounts.json")
	masterPath, activePath := defaultCredsSetup(t, tempDir)

	writeFile(t, targetPath, `{not-json}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.RotateGemini(targetPath, masterPath, activePath)
	if err == nil {
		t.Fatal("RotateGemini() error = nil, want error")
	}

	assertContains(t, err.Error(), "decode gemini target")
}

func TestRotateGeminiErrorsForMissingActive(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "google_accounts.json")
	masterPath, activePath := defaultCredsSetup(t, tempDir)

	writeFile(t, targetPath, `{"old": ["a@example.com"]}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.RotateGemini(targetPath, masterPath, activePath)
	if err == nil {
		t.Fatal("RotateGemini() error = nil, want error")
	}

	assertContains(t, err.Error(), "missing active")
}

func TestRotateGeminiErrorsForEmptyActive(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "google_accounts.json")
	masterPath, activePath := defaultCredsSetup(t, tempDir)

	writeFile(t, targetPath, `{"active": "  ", "old": ["a@example.com"]}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.RotateGemini(targetPath, masterPath, activePath)
	if err == nil {
		t.Fatal("RotateGemini() error = nil, want error")
	}

	assertContains(t, err.Error(), "missing active")
}

func TestRotateGeminiLeavesOriginalFileUntouchedOnWriteFailure(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "google_accounts.json")
	masterPath, activePath := defaultCredsSetup(t, tempDir)

	original := `{
  "active": "first@example.com",
  "old": ["second@example.com"]
}`
	writeFile(t, targetPath, original)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))
	svc.writeFileAtomic = func(string, []byte, os.FileMode) error {
		return errors.New("boom")
	}

	_, err := svc.RotateGemini(targetPath, masterPath, activePath)
	if err == nil {
		t.Fatal("RotateGemini() error = nil, want error")
	}

	assertContains(t, err.Error(), "write gemini target")
	if got := readFile(t, targetPath); got != original {
		t.Fatalf("target file changed unexpectedly\n got: %s\nwant: %s", got, original)
	}
}

func TestRotateGeminiPreservesOrderInOldList(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "google_accounts.json")
	masterPath, activePath := defaultCredsSetup(t, tempDir)

	writeFile(t, targetPath, `{
  "active": "a@example.com",
  "old": ["b@example.com", "c@example.com", "d@example.com"]
}`)

	entries := []GeminiCredEntry{
		{Email: "a@example.com", Data: json.RawMessage(`{"access_token": "a_token"}`)},
		{Email: "b@example.com", Data: json.RawMessage(`{"access_token": "b_token"}`)},
		{Email: "c@example.com", Data: json.RawMessage(`{"access_token": "c_token"}`)},
		{Email: "d@example.com", Data: json.RawMessage(`{"access_token": "d_token"}`)},
	}
	writeMasterCreds(t, masterPath, entries)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.RotateGemini(targetPath, masterPath, activePath)
	if err != nil {
		t.Fatalf("RotateGemini() error = %v", err)
	}

	updated := readFile(t, targetPath)

	var accounts GeminiAccounts
	if err := json.Unmarshal([]byte(updated), &accounts); err != nil {
		t.Fatalf("decode updated file: %v", err)
	}

	if accounts.Active != "b@example.com" {
		t.Fatalf("active = %q, want %q", accounts.Active, "b@example.com")
	}

	expectedOld := []string{"c@example.com", "d@example.com", "a@example.com"}
	if len(accounts.Old) != len(expectedOld) {
		t.Fatalf("old length = %d, want %d. old = %v", len(accounts.Old), len(expectedOld), accounts.Old)
	}
	for i, want := range expectedOld {
		if accounts.Old[i] != want {
			t.Fatalf("old[%d] = %q, want %q", i, accounts.Old[i], want)
		}
	}

	activeCreds := readFile(t, activePath)
	assertContains(t, activeCreds, "b_token")
}

func TestRotateGeminiErrorsForMissingEmailInMasterCreds(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "google_accounts.json")
	masterPath := filepath.Join(tempDir, "oauth_cred_gemini.json")
	activePath := filepath.Join(tempDir, "oauth_creds.json")

	writeFile(t, targetPath, `{
  "active": "a@example.com",
  "old": ["b@example.com"]
}`)

	entries := []GeminiCredEntry{
		{Email: "a@example.com", Data: json.RawMessage(`{"access_token": "a_token"}`)},
	}
	writeMasterCreds(t, masterPath, entries)
	writeActiveCreds(t, activePath, `{"access_token": "a_token"}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.RotateGemini(targetPath, masterPath, activePath)
	if err == nil {
		t.Fatal("RotateGemini() error = nil, want error")
	}

	assertContains(t, err.Error(), "not found in master creds")
	assertContains(t, err.Error(), "***@example.com")
}

func TestRotateGeminiErrorsForMalformedMasterCreds(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "google_accounts.json")
	masterPath := filepath.Join(tempDir, "oauth_cred_gemini.json")
	activePath := filepath.Join(tempDir, "oauth_creds.json")

	writeFile(t, targetPath, `{
  "active": "a@example.com",
  "old": ["b@example.com"]
}`)

	writeFile(t, masterPath, `{not-json}`)
	writeActiveCreds(t, activePath, `{"access_token": "a_token"}`)

	svc := NewService(log.New(&bytes.Buffer{}, "", 0))

	_, err := svc.RotateGemini(targetPath, masterPath, activePath)
	if err == nil {
		t.Fatal("RotateGemini() error = nil, want error")
	}

	assertContains(t, err.Error(), "decode master creds")
}

func TestRotateGeminiCredsWriteFailure(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "google_accounts.json")
	masterPath, activePath := defaultCredsSetup(t, tempDir)

	writeFile(t, targetPath, `{
  "active": "first@example.com",
  "old": ["second@example.com"]
}`)

	entries := []GeminiCredEntry{
		{Email: "first@example.com", Data: json.RawMessage(`{"access_token": "first_token"}`)},
		{Email: "second@example.com", Data: json.RawMessage(`{"access_token": "second_token"}`)},
	}
	writeMasterCreds(t, masterPath, entries)
	writeActiveCreds(t, activePath, `{"access_token": "first_token"}`)

	callCount := 0
	svc := NewService(log.New(&bytes.Buffer{}, "", 0))
	svc.writeFileAtomic = func(path string, data []byte, mode os.FileMode) error {
		callCount++
		if callCount == 1 {
			return writeFileAtomic(path, data, mode)
		}
		return errors.New("creds write failed")
	}

	_, err := svc.RotateGemini(targetPath, masterPath, activePath)
	if err == nil {
		t.Fatal("RotateGemini() error = nil, want error")
	}

	assertContains(t, err.Error(), "write active creds")
}
