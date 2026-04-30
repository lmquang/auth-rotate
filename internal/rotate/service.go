package rotate

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
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
	promptInput     func(string) (string, error)
}

func NewService(logger *log.Logger) *Service {
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	return &Service{
		logger:          logger,
		writeFileAtomic: writeFileAtomic,
		promptInput:     promptInput,
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

func (s *Service) ImportOpenCode(configPath, openAITargetPath, codexTargetPath string) (OpenAIAndCodexResult, error) {
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

	if err := s.writeOpenAIAndCodexTargets(selectedAccount, openAITargetPath, codexTargetPath); err != nil {
		return OpenAIAndCodexResult{}, err
	}

	s.debug("import_opencode complete selected_email=%s", maskEmail(selectedAccount.Email))

	return OpenAIAndCodexResult{
		PreviousEmail: previousEmail,
		SelectedEmail: selectedAccount.Email,
		AccountCount:  len(creds.OpenAICodex.Accounts),
	}, nil
}

func (s *Service) ImportCodex(configPath, openAITargetPath, codexTargetPath string) (OpenAIAndCodexResult, error) {
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
	if err != nil && !isOpenAICodexAccountMissing(err) {
		return OpenAIAndCodexResult{}, err
	}

	previousEmail := creds.OpenAICodex.ActiveEmail
	selectedAccount := OpenAICodexEntry{}
	if matchedIndex == -1 {
		selectedEmail, err := s.codexAccountEmail(codexData, accountID)
		if err != nil {
			return OpenAIAndCodexResult{}, err
		}
		selectedAccount.Email = selectedEmail
		selectedAccount.IsActive = true
	} else {
		selectedAccount = creds.OpenAICodex.Accounts[matchedIndex]
	}
	selectedAccount.AccountID = accountID
	selectedAccount.Codex = codexData
	updatedOpenAI, err := mergeCodexIntoOpenCode(codexData, selectedAccount.OpenAI)
	if err != nil {
		return OpenAIAndCodexResult{}, err
	}
	updatedOpenAI, err = syncOpenCodeExpiresFromCodex(updatedOpenAI, codexData)
	if err != nil {
		return OpenAIAndCodexResult{}, err
	}
	selectedAccount.OpenAI = updatedOpenAI
	if matchedIndex == -1 {
		creds.OpenAICodex.Accounts = append(creds.OpenAICodex.Accounts, selectedAccount)
	} else {
		creds.OpenAICodex.Accounts[matchedIndex] = selectedAccount
	}
	creds.OpenAICodex.ActiveEmail = selectedAccount.Email

	if err := s.saveCredentials(configPath, creds); err != nil {
		return OpenAIAndCodexResult{}, err
	}

	if err := s.writeOpenAIAndCodexTargets(selectedAccount, openAITargetPath, codexTargetPath); err != nil {
		return OpenAIAndCodexResult{}, err
	}

	s.debug("import_codex complete selected_email=%s", maskEmail(selectedAccount.Email))

	return OpenAIAndCodexResult{
		PreviousEmail: previousEmail,
		SelectedEmail: selectedAccount.Email,
		AccountCount:  len(creds.OpenAICodex.Accounts),
	}, nil
}

func isOpenAICodexAccountMissing(err error) bool {
	return err.Error() == "no accounts in openai_codex config" ||
		strings.HasPrefix(err.Error(), "openai_codex account not found for account id: ")
}

func (s *Service) codexAccountEmail(codexData json.RawMessage, accountID string) (string, error) {
	email, err := extractCodexEmail(codexData)
	if err == nil && email != "" {
		return email, nil
	}

	if s.promptInput == nil {
		return "", errors.New("prompt input unavailable")
	}

	email, err = s.promptInput(fmt.Sprintf("Enter email for new Codex account %s: ", accountID))
	if err != nil {
		return "", fmt.Errorf("prompt codex account email: %w", err)
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return "", errors.New("codex account email is empty")
	}

	return email, nil
}

func extractCodexEmail(codexData json.RawMessage) (string, error) {
	var payload struct {
		Email  string `json:"email"`
		Tokens struct {
			IDToken string `json:"id_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(codexData, &payload); err != nil {
		return "", fmt.Errorf("decode codex email: %w", err)
	}
	if payload.Email != "" {
		return payload.Email, nil
	}
	if payload.Tokens.IDToken == "" {
		return "", nil
	}

	parts := strings.Split(payload.Tokens.IDToken, ".")
	if len(parts) < 2 {
		return "", nil
	}

	claimsData, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(claimsData, &claims); err != nil {
		return "", nil
	}

	return claims.Email, nil
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

func openCodeFromCodex(codexData json.RawMessage) (json.RawMessage, error) {
	var codexPayload struct {
		Tokens struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			AccountID    string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(codexData, &codexPayload); err != nil {
		return nil, fmt.Errorf("decode codex tokens for openai: %w", err)
	}

	openAIPayload := map[string]any{
		"type":      "oauth",
		"accountId": codexPayload.Tokens.AccountID,
	}
	if codexPayload.Tokens.AccessToken != "" {
		openAIPayload["access"] = codexPayload.Tokens.AccessToken
	}
	if codexPayload.Tokens.RefreshToken != "" {
		openAIPayload["refresh"] = codexPayload.Tokens.RefreshToken
	}

	openAIJSON, err := json.Marshal(openAIPayload)
	if err != nil {
		return nil, fmt.Errorf("encode openai target from codex: %w", err)
	}

	return json.RawMessage(openAIJSON), nil
}

func mergeCodexIntoOpenCode(codexData, existingOpenAI json.RawMessage) (json.RawMessage, error) {
	openAIData, err := openCodeFromCodex(codexData)
	if err != nil {
		return nil, err
	}

	var openAIPayload map[string]any
	if len(existingOpenAI) > 0 {
		if err := json.Unmarshal(existingOpenAI, &openAIPayload); err != nil {
			return nil, fmt.Errorf("decode existing openai target: %w", err)
		}
	}
	if openAIPayload == nil {
		openAIPayload = make(map[string]any)
	}

	var codexOpenAIPayload map[string]any
	if err := json.Unmarshal(openAIData, &codexOpenAIPayload); err != nil {
		return nil, fmt.Errorf("decode generated openai target: %w", err)
	}
	for key, value := range codexOpenAIPayload {
		openAIPayload[key] = value
	}

	updatedOpenAIJSON, err := json.Marshal(openAIPayload)
	if err != nil {
		return nil, fmt.Errorf("encode openai target from codex: %w", err)
	}

	return json.RawMessage(updatedOpenAIJSON), nil
}

func syncOpenCodeExpiresFromCodex(openAIData, codexData json.RawMessage) (json.RawMessage, error) {
	if len(openAIData) == 0 {
		return openAIData, nil
	}

	var openAIPayload map[string]any
	if err := json.Unmarshal(openAIData, &openAIPayload); err != nil {
		return nil, fmt.Errorf("decode openai target for expires: %w", err)
	}

	expires := time.Now().Add(7 * 24 * time.Hour).UnixMilli()
	if accessToken, err := extractCodexAccessToken(codexData); err != nil {
		return nil, err
	} else if tokenExpires, ok := jwtExpiresUnixMilli(accessToken); ok {
		expires = tokenExpires
	}
	openAIPayload["expires"] = expires

	updatedOpenAIJSON, err := json.Marshal(openAIPayload)
	if err != nil {
		return nil, fmt.Errorf("encode openai target with expires: %w", err)
	}

	return json.RawMessage(updatedOpenAIJSON), nil
}

func extractCodexAccessToken(codexData json.RawMessage) (string, error) {
	var codexPayload struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(codexData, &codexPayload); err != nil {
		return "", fmt.Errorf("decode codex access token: %w", err)
	}

	return codexPayload.Tokens.AccessToken, nil
}

func jwtExpiresUnixMilli(token string) (int64, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return 0, false
	}

	claimsData, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, false
	}

	var claims struct {
		Exp json.Number `json:"exp"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(claimsData)))
	decoder.UseNumber()
	if err := decoder.Decode(&claims); err != nil {
		return 0, false
	}

	expires, err := claims.Exp.Int64()
	if err != nil || expires <= 0 {
		return 0, false
	}

	return expires * int64(time.Second/time.Millisecond), true
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

func promptInput(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	value, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	return value, nil
}
