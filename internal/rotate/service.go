package rotate

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const openAIProviderKey = "openai"

type OpenAIAuth struct {
	Type      string `json:"type"`
	Refresh   string `json:"refresh"`
	Access    string `json:"access"`
	Expires   int64  `json:"expires"`
	AccountID string `json:"accountId"`
}

type Account struct {
	Email  string     `json:"email"`
	OpenAI OpenAIAuth `json:"openai"`
}

type AuthConfig map[string]json.RawMessage

type Result struct {
	PreviousAccountID string
	SelectedAccountID string
	SelectedEmail     string
	AccountCount      int
}

type Service struct {
	logger          *log.Logger
	writeFileAtomic func(string, []byte, os.FileMode) error
}

func NewService(logger *log.Logger) *Service {
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	return &Service{
		logger:          logger,
		writeFileAtomic: writeFileAtomic,
	}
}

func (s *Service) Rotate(sourcePath string, targetPath string) (Result, error) {
	s.debug("rotate start source=%s target=%s", sourcePath, targetPath)
	unlock, err := lockFile(targetPath + ".lock")
	if err != nil {
		return Result{}, fmt.Errorf("lock auth target: %w", err)
	}
	defer unlock()

	s.debug("rotate lock acquired target=%s", targetPath)

	accounts, err := s.loadAccounts(sourcePath)
	if err != nil {
		return Result{}, err
	}

	targetConfig, currentOpenAI, targetMode, err := s.loadTargetConfig(targetPath)
	if err != nil {
		return Result{}, err
	}

	nextAccount, err := selectNextAccount(accounts, currentOpenAI.AccountID)
	if err != nil {
		return Result{}, err
	}

	openAIJSON, err := json.MarshalIndent(nextAccount.OpenAI, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("encode next openai config: %w", err)
	}

	targetConfig[openAIProviderKey] = openAIJSON

	updatedJSON, err := json.MarshalIndent(targetConfig, "", "  ")
	if err != nil {
		return Result{}, fmt.Errorf("encode auth target: %w", err)
	}
	updatedJSON = append(updatedJSON, '\n')

	s.debug(
		"rotate selected previous_account_id=%s selected_account_id=%s selected_email=%s account_count=%d",
		currentOpenAI.AccountID,
		nextAccount.OpenAI.AccountID,
		maskEmail(nextAccount.Email),
		len(accounts),
	)

	if err := s.writeFileAtomic(targetPath, updatedJSON, targetMode); err != nil {
		return Result{}, fmt.Errorf("write auth target: %w", err)
	}

	s.debug("rotate complete target=%s selected_account_id=%s", targetPath, nextAccount.OpenAI.AccountID)

	return Result{
		PreviousAccountID: currentOpenAI.AccountID,
		SelectedAccountID: nextAccount.OpenAI.AccountID,
		SelectedEmail:     nextAccount.Email,
		AccountCount:      len(accounts),
	}, nil
}

func (s *Service) loadAccounts(path string) ([]Account, error) {
	s.debug("load accounts start path=%s", path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read source accounts: %w", err)
	}

	var accounts []Account
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, fmt.Errorf("decode source accounts: %w", err)
	}

	if len(accounts) == 0 {
		return nil, errors.New("no accounts in source file")
	}

	seenAccountIDs := make(map[string]struct{}, len(accounts))

	for index, account := range accounts {
		if err := validateAccount(account); err != nil {
			return nil, fmt.Errorf("invalid source account at index %d: %w", index, err)
		}

		if _, exists := seenAccountIDs[account.OpenAI.AccountID]; exists {
			return nil, fmt.Errorf("duplicate openai.accountId %q in source accounts", account.OpenAI.AccountID)
		}

		seenAccountIDs[account.OpenAI.AccountID] = struct{}{}
	}

	s.debug("load accounts complete path=%s count=%d", path, len(accounts))
	return accounts, nil
}

func (s *Service) loadTargetConfig(path string) (AuthConfig, OpenAIAuth, os.FileMode, error) {
	s.debug("load target start path=%s", path)

	info, err := os.Stat(path)
	if err != nil {
		return nil, OpenAIAuth{}, 0, fmt.Errorf("stat auth target: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, OpenAIAuth{}, 0, fmt.Errorf("read auth target: %w", err)
	}

	var config AuthConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, OpenAIAuth{}, 0, fmt.Errorf("decode auth target: %w", err)
	}

	openAIJSON, ok := config[openAIProviderKey]
	if !ok {
		return nil, OpenAIAuth{}, 0, errors.New("missing openai config in auth target")
	}

	var openAI OpenAIAuth
	if err := json.Unmarshal(openAIJSON, &openAI); err != nil {
		return nil, OpenAIAuth{}, 0, fmt.Errorf("decode auth target openai: %w", err)
	}

	if strings.TrimSpace(openAI.AccountID) == "" {
		return nil, OpenAIAuth{}, 0, errors.New("missing openai.accountId in auth target")
	}

	s.debug("load target complete path=%s current_account_id=%s", path, openAI.AccountID)
	return config, openAI, info.Mode().Perm(), nil
}

func selectNextAccount(accounts []Account, currentAccountID string) (Account, error) {
	for index, account := range accounts {
		if account.OpenAI.AccountID != currentAccountID {
			continue
		}

		nextIndex := (index + 1) % len(accounts)
		return accounts[nextIndex], nil
	}

	return Account{}, fmt.Errorf("current accountId %q not found in source accounts", currentAccountID)
}

func validateAccount(account Account) error {
	if strings.TrimSpace(account.Email) == "" {
		return errors.New("missing email")
	}

	if strings.TrimSpace(account.OpenAI.Type) == "" {
		return errors.New("missing openai.type")
	}

	if strings.TrimSpace(account.OpenAI.Access) == "" {
		return errors.New("missing openai.access")
	}

	if strings.TrimSpace(account.OpenAI.Refresh) == "" {
		return errors.New("missing openai.refresh")
	}

	if account.OpenAI.Expires == 0 {
		return errors.New("missing openai.expires")
	}

	if strings.TrimSpace(account.OpenAI.AccountID) == "" {
		return errors.New("missing openai.accountId")
	}

	return nil
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".auth.json.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmpFile.Chmod(perm); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	cleanup = false
	return nil
}

func lockFile(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}

	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

func maskEmail(value string) string {
	parts := strings.Split(value, "@")
	if len(parts) != 2 {
		return "***"
	}

	local := parts[0]
	if len(local) <= 2 {
		return "***@" + parts[1]
	}

	return local[:1] + "***" + local[len(local)-1:] + "@" + parts[1]
}

func (s *Service) debug(format string, args ...any) {
	if s.logger == nil {
		return
	}

	s.logger.Printf("DEBUG "+format, args...)
}
