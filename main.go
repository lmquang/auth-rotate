package main

import (
	"auth-rotate/internal/rotate"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Printf("DEBUG cli error resolving_home err=%v", err)
		fmt.Fprintf(os.Stderr, "resolve home directory failed: %v\n", err)
		os.Exit(1)
	}

	defaultSourcePath := filepath.Join(homeDir, ".local", "share", "opencode", "auth_open_ai.json")
	defaultTargetPath := filepath.Join(homeDir, ".local", "share", "opencode", "auth.json")
	defaultGeminiPath := filepath.Join(homeDir, ".gemini", "google_accounts.json")

	provider := flag.String("provider", "openai", "provider to rotate: openai or gemini")
	sourcePath := flag.String("source", defaultSourcePath, "path to auth_open_ai.json (openai only)")
	targetPath := flag.String("target", defaultTargetPath, "path to auth.json (openai only)")
	geminiPath := flag.String("gemini-path", defaultGeminiPath, "path to google_accounts.json (gemini only)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: auth-rotate [options]\n\n")
		fmt.Fprintf(os.Stderr, "Round-robin rotation of OAuth accounts.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  auth-rotate                              # rotate openai (default)\n")
		fmt.Fprintf(os.Stderr, "  auth-rotate -provider gemini             # rotate gemini\n")
		fmt.Fprintf(os.Stderr, "  auth-rotate -provider openai -source ./accounts.json -target ./auth.json\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	logger.Printf("DEBUG cli start provider=%s", *provider)

	service := rotate.NewService(logger)

	switch *provider {
	case "openai":
		result, err := service.Rotate(*sourcePath, *targetPath)
		if err != nil {
			logger.Printf("DEBUG cli error source=%s target=%s err=%v", *sourcePath, *targetPath, err)
			fmt.Fprintf(os.Stderr, "rotate failed: %v\n", err)
			os.Exit(1)
		}

		logger.Printf(
			"DEBUG cli complete source=%s target=%s previous_account_id=%s selected_account_id=%s account_count=%d",
			*sourcePath,
			*targetPath,
			result.PreviousAccountID,
			result.SelectedAccountID,
			result.AccountCount,
		)

		fmt.Printf("rotated openai account: %s -> %s\n", result.PreviousAccountID, result.SelectedAccountID)

	case "gemini":
		geminiDir := filepath.Dir(*geminiPath)
		masterCredsPath := filepath.Join(geminiDir, "oauth_cred_gemini.json")
		activeCredsPath := filepath.Join(geminiDir, "oauth_creds.json")

		logger.Printf("DEBUG cli gemini paths google_accounts=%s master_creds=%s active_creds=%s",
			*geminiPath, masterCredsPath, activeCredsPath)

		result, err := service.RotateGemini(*geminiPath, masterCredsPath, activeCredsPath)
		if err != nil {
			logger.Printf("DEBUG cli error gemini_path=%s err=%v", *geminiPath, err)
			fmt.Fprintf(os.Stderr, "rotate gemini failed: %v\n", err)
			os.Exit(1)
		}

		logger.Printf(
			"DEBUG cli complete gemini_path=%s previous_email=%s selected_email=%s account_count=%d",
			*geminiPath,
			result.PreviousEmail,
			result.SelectedEmail,
			result.AccountCount,
		)

		fmt.Printf("rotated gemini account: %s -> %s\n", result.PreviousEmail, result.SelectedEmail)

	default:
		logger.Printf("DEBUG cli error unknown_provider=%s", *provider)
		fmt.Fprintf(os.Stderr, "unknown provider: %s (use openai or gemini)\n", *provider)
		os.Exit(1)
	}
}
