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

	configData, err := os.ReadFile(configPath)
	if err != nil {
		return OpenAIAndCodexResult{}, fmt.Errorf("read config: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(configData, &creds); err != nil {
		return OpenAIAndCodexResult{}, fmt.Errorf("decode config: %w", err)
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

	if selectedAccount.OpenAI != nil && len(selectedAccount.OpenAI) > 0 {
		if err := s.updateTargetNode(openAITargetPath, "openai", selectedAccount.OpenAI); err != nil {
			return OpenAIAndCodexResult{}, fmt.Errorf("update openai target: %w", err)
		}
	}

	if selectedAccount.Codex != nil && len(selectedAccount.Codex) > 0 {
		var codexObj map[string]any
		if err := json.Unmarshal(selectedAccount.Codex, &codexObj); err == nil {
			updatedCodexJSON, _ := json.MarshalIndent(codexObj, "", "  ")
			updatedCodexJSON = append(updatedCodexJSON, '\n')
			if err := s.writeFileAtomic(codexTargetPath, updatedCodexJSON, 0o600); err != nil {
				return OpenAIAndCodexResult{}, fmt.Errorf("write codex target: %w", err)
			}
		}
	}

	s.debug("rotate_openai_codex complete selected_email=%s", maskEmail(selectedAccount.Email))

	return OpenAIAndCodexResult{
		PreviousEmail: previousEmail,
		SelectedEmail: selectedAccount.Email,
		AccountCount:  accountCount,
	}, nil
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
