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

type OpenAIAndCodexResult struct {
	PreviousEmail string
	SelectedEmail string
	AccountCount  int
}

type GeminiResult struct {
	PreviousEmail string
	SelectedEmail string
	AccountCount  int
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

func (s *Service) RotateOpenAIAndCodex(configPath, openAITargetPath, codexTargetPath string) (OpenAIAndCodexResult, error) {
	s.debug("rotate_openai_codex start config=%s target_openai=%s target_codex=%s", configPath, openAITargetPath, codexTargetPath)

	unlock, err := lockFile(configPath + ".lock")
	if err != nil {
		return OpenAIAndCodexResult{}, fmt.Errorf("lock config: %w", err)
	}
	defer unlock()

	creds, err := loadCredentials(configPath)
	if err != nil {
		return OpenAIAndCodexResult{}, err
	}

	if len(creds.OpenAICodex.Accounts) == 0 {
		return OpenAIAndCodexResult{}, errors.New("no accounts in openai_codex config")
	}

	previousEmail := creds.OpenAICodex.ActiveEmail
	var selectedAccount *OpenAICodexEntry

	currentIndex := 0
	if previousEmail != "" {
		for i, acc := range creds.OpenAICodex.Accounts {
			if acc.Email == previousEmail {
				currentIndex = i
				break
			}
		}
	}

	accountCount := len(creds.OpenAICodex.Accounts)
	for i := 1; i <= accountCount; i++ {
		nextIndex := (currentIndex + i) % accountCount
		acc := creds.OpenAICodex.Accounts[nextIndex]
		if acc.IsActive {
			selectedAccount = &acc
			break
		}
	}

	if selectedAccount == nil {
		return OpenAIAndCodexResult{}, errors.New("no active accounts found in openai_codex config")
	}

	creds.OpenAICodex.ActiveEmail = selectedAccount.Email

	updatedConfigJSON, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return OpenAIAndCodexResult{}, fmt.Errorf("encode updated config: %w", err)
	}
	updatedConfigJSON = append(updatedConfigJSON, '\n')

	if err := s.writeFileAtomic(configPath, updatedConfigJSON, 0o600); err != nil {
		return OpenAIAndCodexResult{}, fmt.Errorf("write config target: %w", err)
	}

	if err := s.writeOpenAIAndCodexTargets(*selectedAccount, openAITargetPath, codexTargetPath); err != nil {
		return OpenAIAndCodexResult{}, err
	}

	s.debug("rotate_openai_codex complete selected_email=%s", maskEmail(selectedAccount.Email))

	return OpenAIAndCodexResult{
		PreviousEmail: previousEmail,
		SelectedEmail: selectedAccount.Email,
		AccountCount:  accountCount,
	}, nil
}

func (s *Service) SyncOpenAIAndCodex(configPath, openAITargetPath, codexTargetPath string) (OpenAIAndCodexResult, error) {
	s.debug("sync_openai_codex start config=%s target_openai=%s target_codex=%s", configPath, openAITargetPath, codexTargetPath)

	unlock, err := lockFile(configPath + ".lock")
	if err != nil {
		return OpenAIAndCodexResult{}, fmt.Errorf("lock config: %w", err)
	}
	defer unlock()

	creds, err := loadCredentials(configPath)
	if err != nil {
		return OpenAIAndCodexResult{}, err
	}

	selectedAccount, err := findOpenAICodexAccountByEmail(creds.OpenAICodex.Accounts, creds.OpenAICodex.ActiveEmail)
	if err != nil {
		return OpenAIAndCodexResult{}, err
	}

	if err := s.writeOpenAIAndCodexTargets(selectedAccount, openAITargetPath, codexTargetPath); err != nil {
		return OpenAIAndCodexResult{}, err
	}

	s.debug("sync_openai_codex complete selected_email=%s", maskEmail(selectedAccount.Email))

	return OpenAIAndCodexResult{
		PreviousEmail: selectedAccount.Email,
		SelectedEmail: selectedAccount.Email,
		AccountCount:  len(creds.OpenAICodex.Accounts),
	}, nil
}

func (s *Service) ImportOpenCode(configPath, openAITargetPath string) (OpenAIAndCodexResult, error) {
	s.debug("import_opencode start config=%s target_openai=%s", configPath, openAITargetPath)

	unlock, err := lockFile(configPath + ".lock")
	if err != nil {
		return OpenAIAndCodexResult{}, fmt.Errorf("lock config: %w", err)
	}
	defer unlock()

	creds, err := loadCredentials(configPath)
	if err != nil {
		return OpenAIAndCodexResult{}, err
	}

	openAIData, accountID, err := loadOpenCodeTarget(openAITargetPath)
	if err != nil {
		return OpenAIAndCodexResult{}, err
	}

	matchedIndex, err := findOpenAICodexAccountIndexByID(creds.OpenAICodex.Accounts, accountID)
	if err != nil {
		return OpenAIAndCodexResult{}, err
	}

	previousEmail := creds.OpenAICodex.ActiveEmail
	selectedAccount := creds.OpenAICodex.Accounts[matchedIndex]
	selectedAccount.AccountID = accountID
	selectedAccount.OpenAI = openAIData
	updatedCodex, err := mergeOpenCodeIntoCodex(openAIData, selectedAccount.Codex)
	if err != nil {
		return OpenAIAndCodexResult{}, err
	}
	selectedAccount.Codex = updatedCodex
	creds.OpenAICodex.Accounts[matchedIndex] = selectedAccount
	creds.OpenAICodex.ActiveEmail = selectedAccount.Email

	if err := s.saveCredentials(configPath, creds); err != nil {
		return OpenAIAndCodexResult{}, err
	}

	s.debug("import_opencode complete selected_email=%s", maskEmail(selectedAccount.Email))

	return OpenAIAndCodexResult{
		PreviousEmail: previousEmail,
		SelectedEmail: selectedAccount.Email,
		AccountCount:  len(creds.OpenAICodex.Accounts),
	}, nil
}

func (s *Service) ImportCodex(configPath, codexTargetPath string) (OpenAIAndCodexResult, error) {
	s.debug("import_codex start config=%s target_codex=%s", configPath, codexTargetPath)

	unlock, err := lockFile(configPath + ".lock")
	if err != nil {
		return OpenAIAndCodexResult{}, fmt.Errorf("lock config: %w", err)
	}
	defer unlock()

	creds, err := loadCredentials(configPath)
	if err != nil {
		return OpenAIAndCodexResult{}, err
	}

	codexData, accountID, err := loadCodexTarget(codexTargetPath)
	if err != nil {
		return OpenAIAndCodexResult{}, err
	}

	matchedIndex, err := findOpenAICodexAccountIndexByID(creds.OpenAICodex.Accounts, accountID)
	if err != nil {
		return OpenAIAndCodexResult{}, err
	}

	previousEmail := creds.OpenAICodex.ActiveEmail
	selectedAccount := creds.OpenAICodex.Accounts[matchedIndex]
	selectedAccount.AccountID = accountID
	selectedAccount.Codex = codexData
	creds.OpenAICodex.Accounts[matchedIndex] = selectedAccount
	creds.OpenAICodex.ActiveEmail = selectedAccount.Email

	if err := s.saveCredentials(configPath, creds); err != nil {
		return OpenAIAndCodexResult{}, err
	}

	s.debug("import_codex complete selected_email=%s", maskEmail(selectedAccount.Email))

	return OpenAIAndCodexResult{
		PreviousEmail: previousEmail,
		SelectedEmail: selectedAccount.Email,
		AccountCount:  len(creds.OpenAICodex.Accounts),
	}, nil
}

func loadCredentials(configPath string) (Credentials, error) {
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return Credentials{}, fmt.Errorf("read config: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(configData, &creds); err != nil {
		return Credentials{}, fmt.Errorf("decode config: %w", err)
	}

	return creds, nil
}

func (s *Service) saveCredentials(configPath string, creds Credentials) error {
	updatedConfigJSON, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("encode updated config: %w", err)
	}
	updatedConfigJSON = append(updatedConfigJSON, '\n')

	if err := s.writeFileAtomic(configPath, updatedConfigJSON, 0o600); err != nil {
		return fmt.Errorf("write config target: %w", err)
	}

	return nil
}

func findOpenAICodexAccountByEmail(accounts []OpenAICodexEntry, email string) (OpenAICodexEntry, error) {
	if len(accounts) == 0 {
		return OpenAICodexEntry{}, errors.New("no accounts in openai_codex config")
	}

	if email == "" {
		return OpenAICodexEntry{}, errors.New("active openai_codex account is empty")
	}

	for _, account := range accounts {
		if account.Email == email {
			return account, nil
		}
	}

	return OpenAICodexEntry{}, fmt.Errorf("active openai_codex account not found: %s", email)
}

func findOpenAICodexAccountIndexByID(accounts []OpenAICodexEntry, accountID string) (int, error) {
	if len(accounts) == 0 {
		return -1, errors.New("no accounts in openai_codex config")
	}

	if accountID == "" {
		return -1, errors.New("account id is empty")
	}

	matchedIndex := -1
	for i, account := range accounts {
		storedAccountID, err := storedOpenAICodexAccountID(account)
		if err != nil {
			return -1, fmt.Errorf("read stored account id for %s: %w", account.Email, err)
		}

		if storedAccountID != accountID {
			continue
		}

		if matchedIndex != -1 {
			return -1, fmt.Errorf("multiple openai_codex accounts found for account id: %s", accountID)
		}

		matchedIndex = i
	}

	if matchedIndex == -1 {
		return -1, fmt.Errorf("openai_codex account not found for account id: %s", accountID)
	}

	return matchedIndex, nil
}

func storedOpenAICodexAccountID(account OpenAICodexEntry) (string, error) {
	if account.AccountID != "" {
		return account.AccountID, nil
	}

	accountID, err := extractOpenCodeAccountID(account.OpenAI)
	if err != nil {
		return "", err
	}
	if accountID != "" {
		return accountID, nil
	}

	return extractCodexAccountID(account.Codex)
}

func loadOpenCodeTarget(path string) (json.RawMessage, string, error) {
	targetData, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read openai target: %w", err)
	}

	var target struct {
		OpenAI json.RawMessage `json:"openai"`
	}
	if err := json.Unmarshal(targetData, &target); err != nil {
		return nil, "", fmt.Errorf("decode openai target: %w", err)
	}

	if len(target.OpenAI) == 0 {
		return nil, "", errors.New("openai target missing openai credentials")
	}

	accountID, err := extractOpenCodeAccountID(target.OpenAI)
	if err != nil {
		return nil, "", err
	}
	if accountID == "" {
		return nil, "", errors.New("openai target missing accountId")
	}

	return target.OpenAI, accountID, nil
}

func loadCodexTarget(path string) (json.RawMessage, string, error) {
	targetData, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read codex target: %w", err)
	}

	accountID, err := extractCodexAccountID(targetData)
	if err != nil {
		return nil, "", err
	}
	if accountID == "" {
		return nil, "", errors.New("codex target missing tokens.account_id")
	}

	return json.RawMessage(targetData), accountID, nil
}

func extractOpenCodeAccountID(nodeData json.RawMessage) (string, error) {
	if len(nodeData) == 0 {
		return "", nil
	}

	var payload struct {
		AccountID string `json:"accountId"`
	}
	if err := json.Unmarshal(nodeData, &payload); err != nil {
		return "", fmt.Errorf("decode openai account id: %w", err)
	}

	return payload.AccountID, nil
}

func extractCodexAccountID(nodeData json.RawMessage) (string, error) {
	if len(nodeData) == 0 {
		return "", nil
	}

	var payload struct {
		Tokens struct {
			AccountID string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(nodeData, &payload); err != nil {
		return "", fmt.Errorf("decode codex account id: %w", err)
	}

	return payload.Tokens.AccountID, nil
}

func mergeOpenCodeIntoCodex(openAIData, existingCodex json.RawMessage) (json.RawMessage, error) {
	if len(openAIData) == 0 {
		return existingCodex, nil
	}

	var openAIPayload struct {
		Access    string `json:"access"`
		Refresh   string `json:"refresh"`
		AccountID string `json:"accountId"`
	}
	if err := json.Unmarshal(openAIData, &openAIPayload); err != nil {
		return nil, fmt.Errorf("decode openai tokens for codex: %w", err)
	}

	var codexPayload map[string]any
	if len(existingCodex) > 0 {
		if err := json.Unmarshal(existingCodex, &codexPayload); err != nil {
			return nil, fmt.Errorf("decode existing codex target: %w", err)
		}
	}
	if codexPayload == nil {
		codexPayload = make(map[string]any)
	}

	tokens, _ := codexPayload["tokens"].(map[string]any)
	if tokens == nil {
		tokens = make(map[string]any)
	}

	if openAIPayload.Access != "" {
		tokens["access_token"] = openAIPayload.Access
	}
	if openAIPayload.Refresh != "" {
		tokens["refresh_token"] = openAIPayload.Refresh
	}
	if openAIPayload.AccountID != "" {
		tokens["account_id"] = openAIPayload.AccountID
	}

	codexPayload["tokens"] = tokens

	updatedCodexJSON, err := json.Marshal(codexPayload)
	if err != nil {
		return nil, fmt.Errorf("encode codex target from openai: %w", err)
	}

	return json.RawMessage(updatedCodexJSON), nil
}

func (s *Service) writeOpenAIAndCodexTargets(account OpenAICodexEntry, openAITargetPath, codexTargetPath string) error {
	if account.OpenAI != nil && len(account.OpenAI) > 0 {
		if err := s.updateTargetNode(openAITargetPath, "openai", account.OpenAI); err != nil {
			return fmt.Errorf("update openai target: %w", err)
		}
	}

	if err := s.writeCodexTarget(codexTargetPath, account.Codex); err != nil {
		return err
	}

	return nil
}

func (s *Service) writeCodexTarget(path string, nodeData json.RawMessage) error {
	if len(nodeData) == 0 {
		return nil
	}

	var codexObj map[string]any
	if err := json.Unmarshal(nodeData, &codexObj); err != nil {
		return fmt.Errorf("decode codex target: %w", err)
	}

	updatedCodexJSON, err := json.MarshalIndent(codexObj, "", "  ")
	if err != nil {
		return fmt.Errorf("encode codex target: %w", err)
	}
	updatedCodexJSON = append(updatedCodexJSON, '\n')

	if err := s.writeFileAtomic(path, updatedCodexJSON, 0o600); err != nil {
		return fmt.Errorf("write codex target: %w", err)
	}

	return nil
}

func (s *Service) updateTargetNode(path, jsonKey string, nodeData json.RawMessage) error {
	var targetConfig map[string]json.RawMessage
	targetMode := os.FileMode(0o600)

	if info, err := os.Stat(path); err == nil {
		targetMode = info.Mode().Perm()
		if targetData, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(targetData, &targetConfig)
		}
	}

	if targetConfig == nil {
		targetConfig = make(map[string]json.RawMessage)
	}

	targetConfig[jsonKey] = nodeData

	updatedJSON, err := json.MarshalIndent(targetConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("encode target file: %w", err)
	}
	updatedJSON = append(updatedJSON, '\n')

	if err := s.writeFileAtomic(path, updatedJSON, targetMode); err != nil {
		return fmt.Errorf("write target file: %w", err)
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
