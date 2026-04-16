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

	defaultConfigPath := filepath.Join(homeDir, ".config", "auth-rotate", "credentials.json")
	defaultOpenAITargetPath := filepath.Join(homeDir, ".local", "share", "opencode", "auth.json")
	defaultCodexTargetPath := filepath.Join(homeDir, ".codex", "auth.json")
	defaultGeminiActiveCredsPath := filepath.Join(homeDir, ".gemini", "oauth_creds.json")

	provider := flag.String("provider", "openai", "provider to rotate: openai or gemini")
	configPath := flag.String("config", defaultConfigPath, "path to credentials.json")
	openAITargetPath := flag.String("openai-target", defaultOpenAITargetPath, "path to opencode auth.json")
	codexTargetPath := flag.String("codex-target", defaultCodexTargetPath, "path to codex auth.json")
	geminiActivePath := flag.String("gemini-target", defaultGeminiActiveCredsPath, "path to gemini oauth_creds.json")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: auth-rotate [options]\n\n")
		fmt.Fprintf(os.Stderr, "Round-robin rotation of OAuth accounts from a central config.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  auth-rotate                              # rotate openai and codex (default)\n")
		fmt.Fprintf(os.Stderr, "  auth-rotate -provider gemini             # rotate gemini\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	logger.Printf("DEBUG cli start provider=%s", *provider)

	service := rotate.NewService(logger)

	switch *provider {
	case "openai":
		result, err := service.RotateOpenAIAndCodex(*configPath, *openAITargetPath, *codexTargetPath)
		if err != nil {
			logger.Printf("DEBUG cli error config=%s target=%s err=%v", *configPath, *openAITargetPath, err)
			fmt.Fprintf(os.Stderr, "rotate openai and codex failed: %v\n", err)
			os.Exit(1)
		}

		logger.Printf(
			"DEBUG cli complete previous_email=%s selected_email=%s account_count=%d",
			result.PreviousEmail,
			result.SelectedEmail,
			result.AccountCount,
		)

		fmt.Printf("rotated openai and codex account: %s -> %s\n", result.PreviousEmail, result.SelectedEmail)

	case "gemini":
		result, err := service.RotateGemini(*configPath, *geminiActivePath)
		if err != nil {
			logger.Printf("DEBUG cli error config=%s target=%s err=%v", *configPath, *geminiActivePath, err)
			fmt.Fprintf(os.Stderr, "rotate gemini failed: %v\n", err)
			os.Exit(1)
		}

		logger.Printf(
			"DEBUG cli complete previous_email=%s selected_email=%s account_count=%d",
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
