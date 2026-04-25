package rotate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

func (s *Service) RotateGemini(configPath, activeCredsPath string) (GeminiResult, error) {
	s.debug("rotate_gemini start config=%s target_active=%s", configPath, activeCredsPath)

	unlock, err := lockFile(configPath + ".lock")
	if err != nil {
		return GeminiResult{}, fmt.Errorf("lock config: %w", err)
	}
	defer unlock()

	creds, err := loadCredentials(configPath)
	if err != nil {
		return GeminiResult{}, err
	}

	if len(creds.Gemini.Accounts) == 0 {
		return GeminiResult{}, errors.New("no accounts in gemini config")
	}

	previousEmail := creds.Gemini.ActiveEmail
	var selectedAccount *GeminiCredEntry

	currentIndex := 0
	if previousEmail != "" {
		for i, acc := range creds.Gemini.Accounts {
			if acc.Email == previousEmail {
				currentIndex = i
				break
			}
		}
	}

	accountCount := len(creds.Gemini.Accounts)
	for i := 1; i <= accountCount; i++ {
		nextIndex := (currentIndex + i) % accountCount
		acc := creds.Gemini.Accounts[nextIndex]
		if acc.IsActive {
			selectedAccount = &acc
			break
		}
	}

	if selectedAccount == nil {
		return GeminiResult{}, errors.New("no active accounts found in gemini config")
	}

	creds.Gemini.ActiveEmail = selectedAccount.Email

	updatedConfigJSON, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return GeminiResult{}, fmt.Errorf("encode updated config: %w", err)
	}
	updatedConfigJSON = append(updatedConfigJSON, '\n')

	if err := s.writeFileAtomic(configPath, updatedConfigJSON, 0o600); err != nil {
		return GeminiResult{}, fmt.Errorf("write config target: %w", err)
	}

	if err := s.writeGeminiTarget(activeCredsPath, selectedAccount.Data); err != nil {
		return GeminiResult{}, err
	}

	s.debug("rotate_gemini complete selected_email=%s", maskEmail(selectedAccount.Email))

	return GeminiResult{
		PreviousEmail: previousEmail,
		SelectedEmail: selectedAccount.Email,
		AccountCount:  accountCount,
	}, nil
}

func (s *Service) SyncGemini(configPath, activeCredsPath string) (GeminiResult, error) {
	s.debug("sync_gemini start config=%s target_active=%s", configPath, activeCredsPath)

	unlock, err := lockFile(configPath + ".lock")
	if err != nil {
		return GeminiResult{}, fmt.Errorf("lock config: %w", err)
	}
	defer unlock()

	creds, err := loadCredentials(configPath)
	if err != nil {
		return GeminiResult{}, err
	}

	selectedAccount, err := findGeminiAccountByEmail(creds.Gemini.Accounts, creds.Gemini.ActiveEmail)
	if err != nil {
		return GeminiResult{}, err
	}

	if err := s.writeGeminiTarget(activeCredsPath, selectedAccount.Data); err != nil {
		return GeminiResult{}, err
	}

	s.debug("sync_gemini complete selected_email=%s", maskEmail(selectedAccount.Email))

	return GeminiResult{
		PreviousEmail: selectedAccount.Email,
		SelectedEmail: selectedAccount.Email,
		AccountCount:  len(creds.Gemini.Accounts),
	}, nil
}

func findGeminiAccountByEmail(accounts []GeminiCredEntry, email string) (GeminiCredEntry, error) {
	if len(accounts) == 0 {
		return GeminiCredEntry{}, errors.New("no accounts in gemini config")
	}

	if email == "" {
		return GeminiCredEntry{}, errors.New("active gemini account is empty")
	}

	for _, account := range accounts {
		if account.Email == email {
			return account, nil
		}
	}

	return GeminiCredEntry{}, fmt.Errorf("active gemini account not found: %s", email)
}

func (s *Service) writeGeminiTarget(path string, nodeData json.RawMessage) error {
	info, err := os.Stat(path)
	targetMode := os.FileMode(0o600)
	if err == nil {
		targetMode = info.Mode().Perm()
	}

	activeJSON, err := json.MarshalIndent(nodeData, "", "  ")
	if err != nil {
		return fmt.Errorf("encode active creds: %w", err)
	}
	activeJSON = append(activeJSON, '\n')

	if err := s.writeFileAtomic(path, activeJSON, targetMode); err != nil {
		return fmt.Errorf("write active creds target: %w", err)
	}

	return nil
}
