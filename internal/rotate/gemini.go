package rotate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type GeminiAccounts struct {
	Active string   `json:"active"`
	Old    []string `json:"old"`
}

type GeminiCredEntry struct {
	Email string          `json:"email"`
	Data  json.RawMessage `json:"data"`
}

type GeminiResult struct {
	PreviousEmail string
	SelectedEmail string
	AccountCount  int
}

func (s *Service) RotateGemini(googleAccountsPath, masterCredsPath, activeCredsPath string) (GeminiResult, error) {
	s.debug("rotate_gemini start google_accounts_path=%s master_creds_path=%s active_creds_path=%s",
		googleAccountsPath, masterCredsPath, activeCredsPath)

	unlock, err := lockFile(googleAccountsPath + ".lock")
	if err != nil {
		return GeminiResult{}, fmt.Errorf("lock gemini target: %w", err)
	}
	defer unlock()

	s.debug("rotate_gemini lock acquired path=%s", googleAccountsPath)

	info, err := os.Stat(googleAccountsPath)
	if err != nil {
		return GeminiResult{}, fmt.Errorf("stat gemini target: %w", err)
	}

	data, err := os.ReadFile(googleAccountsPath)
	if err != nil {
		return GeminiResult{}, fmt.Errorf("read gemini target: %w", err)
	}

	var accounts GeminiAccounts
	if err := json.Unmarshal(data, &accounts); err != nil {
		return GeminiResult{}, fmt.Errorf("decode gemini target: %w", err)
	}

	if strings.TrimSpace(accounts.Active) == "" {
		return GeminiResult{}, errors.New("missing active email in gemini accounts")
	}

	previousEmail := accounts.Active

	if len(accounts.Old) == 0 {
		s.debug("rotate_gemini single account stable email=%s", maskEmail(previousEmail))

		if err := s.updateCredsFile(masterCredsPath, activeCredsPath, previousEmail); err != nil {
			return GeminiResult{}, err
		}

		return GeminiResult{
			PreviousEmail: previousEmail,
			SelectedEmail: previousEmail,
			AccountCount:  1,
		}, nil
	}

	nextEmail := accounts.Old[0]

	newOld := make([]string, 0, len(accounts.Old))
	for _, email := range accounts.Old[1:] {
		if email != previousEmail {
			newOld = append(newOld, email)
		}
	}
	if previousEmail != nextEmail {
		newOld = append(newOld, previousEmail)
	}

	updated := GeminiAccounts{
		Active: nextEmail,
		Old:    newOld,
	}

	updatedJSON, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return GeminiResult{}, fmt.Errorf("encode gemini target: %w", err)
	}
	updatedJSON = append(updatedJSON, '\n')

	accountCount := len(accounts.Old) + 1

	s.debug(
		"rotate_gemini selected previous_email=%s selected_email=%s account_count=%d",
		maskEmail(previousEmail),
		maskEmail(nextEmail),
		accountCount,
	)

	if err := s.writeFileAtomic(googleAccountsPath, updatedJSON, info.Mode().Perm()); err != nil {
		return GeminiResult{}, fmt.Errorf("write gemini target: %w", err)
	}

	if err := s.updateCredsFile(masterCredsPath, activeCredsPath, nextEmail); err != nil {
		return GeminiResult{}, err
	}

	s.debug("rotate_gemini complete path=%s selected_email=%s", googleAccountsPath, maskEmail(nextEmail))

	return GeminiResult{
		PreviousEmail: previousEmail,
		SelectedEmail: nextEmail,
		AccountCount:  accountCount,
	}, nil
}

func (s *Service) updateCredsFile(masterCredsPath, activeCredsPath, email string) error {
	s.debug("update_creds start master_path=%s active_path=%s email=%s", masterCredsPath, activeCredsPath, maskEmail(email))

	masterData, err := os.ReadFile(masterCredsPath)
	if err != nil {
		return fmt.Errorf("read master creds: %w", err)
	}

	var entries []GeminiCredEntry
	if err := json.Unmarshal(masterData, &entries); err != nil {
		return fmt.Errorf("decode master creds: %w", err)
	}

	var credData json.RawMessage
	for _, entry := range entries {
		if entry.Email == email {
			credData = entry.Data
			break
		}
	}

	if credData == nil {
		return fmt.Errorf("email %s not found in master creds", maskEmail(email))
	}

	activeJSON, err := json.MarshalIndent(credData, "", "  ")
	if err != nil {
		return fmt.Errorf("encode active creds: %w", err)
	}
	activeJSON = append(activeJSON, '\n')

	info, err := os.Stat(activeCredsPath)
	if err != nil {
		return fmt.Errorf("stat active creds: %w", err)
	}

	if err := s.writeFileAtomic(activeCredsPath, activeJSON, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write active creds: %w", err)
	}

	s.debug("update_creds complete active_path=%s email=%s", activeCredsPath, maskEmail(email))
	return nil
}
