package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type LegacyOpenAIAccount struct {
	Email  string          `json:"email"`
	OpenAI json.RawMessage `json:"openai"`
}

type LegacyCodexAccount struct {
	Email  string          `json:"email"`
	Tokens json.RawMessage `json:"tokens"`
}

type LegacyGeminiAccounts struct {
	Active string   `json:"active"`
	Old    []string `json:"old"`
}

type LegacyGeminiCredEntry struct {
	Email string          `json:"email"`
	Data  json.RawMessage `json:"data"`
}

type NewCredentials struct {
	OpenAICodex struct {
		ActiveEmail string             `json:"activeEmail"`
		Accounts    []OpenAICodexEntry `json:"accounts"`
	} `json:"openai_codex"`

	Gemini struct {
		ActiveEmail string            `json:"activeEmail"`
		Accounts    []GeminiCredEntry `json:"accounts"`
	} `json:"gemini"`
}

type OpenAICodexEntry struct {
	Email    string          `json:"email"`
	IsActive bool            `json:"isActive"`
	OpenAI   json.RawMessage `json:"openai"`
	Codex    json.RawMessage `json:"codex"`
}

type GeminiCredEntry struct {
	Email    string          `json:"email"`
	IsActive bool            `json:"isActive"`
	Data     json.RawMessage `json:"data"`
}

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("resolve home directory failed: %v\n", err)
	}

	openAIPath := filepath.Join(homeDir, ".local", "share", "opencode", "auth_open_ai.json")
	codexPath := filepath.Join(homeDir, ".codex", "auth.json.bak")
	geminiAccountsPath := filepath.Join(homeDir, ".gemini", "google_accounts.json")
	geminiCredsPath := filepath.Join(homeDir, ".gemini", "oauth_cred_gemini.json")
	targetConfigPath := filepath.Join(homeDir, ".config", "auth-rotate", "credentials.json")

	var creds NewCredentials

	// OpenAICodex Sync
	mergedOpenAICodex := make(map[string]*OpenAICodexEntry)

	// Read OpenAI
	if data, err := os.ReadFile(openAIPath); err == nil {
		var list []LegacyOpenAIAccount
		if err := json.Unmarshal(data, &list); err == nil {
			for _, acc := range list {
				mergedOpenAICodex[acc.Email] = &OpenAICodexEntry{
					Email:    acc.Email,
					IsActive: true,
					OpenAI:   acc.OpenAI,
				}
			}
			if len(list) > 0 {
				creds.OpenAICodex.ActiveEmail = list[0].Email
			}
		}
	} else {
		fmt.Printf("Warning: Failed to read %s\n", openAIPath)
	}

	// Read Codex
	if data, err := os.ReadFile(codexPath); err == nil {
		var list []map[string]any
		if err := json.Unmarshal(data, &list); err == nil {
			for _, acc := range list {
				email := ""
				if e, ok := acc["email"].(string); ok {
					email = e
				}
				delete(acc, "email") // remove email from raw codex object

				rawBytes, _ := json.Marshal(acc)

				if existing, ok := mergedOpenAICodex[email]; ok {
					existing.Codex = rawBytes
				} else {
					mergedOpenAICodex[email] = &OpenAICodexEntry{
						Email:    email,
						IsActive: true,
						Codex:    rawBytes,
					}
				}
			}
		}
	} else {
		fmt.Printf("Warning: Failed to read %s\n", codexPath)
	}

	for _, entry := range mergedOpenAICodex {
		creds.OpenAICodex.Accounts = append(creds.OpenAICodex.Accounts, *entry)
	}

	// Gemini Sync
	if accountsData, err := os.ReadFile(geminiAccountsPath); err == nil {
		var legacyAccts LegacyGeminiAccounts
		if err := json.Unmarshal(accountsData, &legacyAccts); err == nil {
			creds.Gemini.ActiveEmail = legacyAccts.Active

			// Parse creds
			if credsData, err := os.ReadFile(geminiCredsPath); err == nil {
				var legacyCreds []LegacyGeminiCredEntry
				if err := json.Unmarshal(credsData, &legacyCreds); err == nil {
					for _, c := range legacyCreds {
						creds.Gemini.Accounts = append(creds.Gemini.Accounts, GeminiCredEntry{
							Email:    c.Email,
							IsActive: true, // Mark all legacy as active by default
							Data:     c.Data,
						})
					}
				}
			} else {
				fmt.Printf("Warning: Failed to read %s\n", geminiCredsPath)
			}
		}
	} else {
		fmt.Printf("Warning: Failed to read %s\n", geminiAccountsPath)
	}

	// Dump out to credentials.json
	targetDir := filepath.Dir(targetConfigPath)
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		log.Fatalf("Failed to create target directory %s: %v\n", targetDir, err)
	}

	outData, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal credentials: %v\n", err)
	}
	outData = append(outData, '\n')

	if err := os.WriteFile(targetConfigPath, outData, 0o600); err != nil {
		log.Fatalf("Failed to write to %s: %v\n", targetConfigPath, err)
	}

	fmt.Printf("Data synchronization successful! Merged credentials written to:\n%s\n", targetConfigPath)
}
