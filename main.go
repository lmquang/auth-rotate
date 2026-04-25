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

	command := "rotate"
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "rotate", "sync", "import":
			command = args[0]
			args = args[1:]
		}
	}

	flags := flag.NewFlagSet("auth-rotate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	provider := flags.String("provider", "openai", "provider to use for the selected command")
	configPath := flags.String("config", defaultConfigPath, "path to credentials.json")
	openAITargetPath := flags.String("openai-target", defaultOpenAITargetPath, "path to opencode auth.json")
	codexTargetPath := flags.String("codex-target", defaultCodexTargetPath, "path to codex auth.json")
	geminiActivePath := flags.String("gemini-target", defaultGeminiActiveCredsPath, "path to gemini oauth_creds.json")

	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: auth-rotate [rotate|sync|import] [options]\n\n")
		fmt.Fprintf(os.Stderr, "Rotate, sync, or import OAuth accounts from a central config.\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  auth-rotate                              # rotate openai and codex (default)\n")
		fmt.Fprintf(os.Stderr, "  auth-rotate rotate -provider gemini      # rotate gemini\n")
		fmt.Fprintf(os.Stderr, "  auth-rotate sync                         # sync current openai and codex account\n")
		fmt.Fprintf(os.Stderr, "  auth-rotate sync -provider gemini        # sync current gemini account\n")
		fmt.Fprintf(os.Stderr, "  auth-rotate import -provider opencode    # import opencode auth into credentials.json\n")
		fmt.Fprintf(os.Stderr, "  auth-rotate import -provider codex       # import codex auth into credentials.json\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		logger.Printf("DEBUG cli error parse_flags err=%v", err)
		os.Exit(2)
	}
	if flags.NArg() > 0 {
		logger.Printf("DEBUG cli error unexpected_args=%d", flags.NArg())
		fmt.Fprintf(os.Stderr, "unexpected arguments: %v\n\n", flags.Args())
		flags.Usage()
		os.Exit(2)
	}

	logger.Printf("DEBUG cli start command=%s provider=%s", command, *provider)

	service := rotate.NewService(logger)

	switch command {
	case "rotate":
		switch *provider {
		case "openai":
			result, err := service.RotateOpenAIAndCodex(*configPath, *openAITargetPath, *codexTargetPath)
			if err != nil {
				logger.Printf("DEBUG cli error command=%s config=%s target=%s err=%v", command, *configPath, *openAITargetPath, err)
				fmt.Fprintf(os.Stderr, "rotate openai and codex failed: %v\n", err)
				os.Exit(1)
			}

			logger.Printf(
				"DEBUG cli complete command=%s previous_email=%s selected_email=%s account_count=%d",
				command,
				result.PreviousEmail,
				result.SelectedEmail,
				result.AccountCount,
			)

			fmt.Printf("rotated openai and codex account: %s -> %s\n", result.PreviousEmail, result.SelectedEmail)

		case "gemini":
			result, err := service.RotateGemini(*configPath, *geminiActivePath)
			if err != nil {
				logger.Printf("DEBUG cli error command=%s config=%s target=%s err=%v", command, *configPath, *geminiActivePath, err)
				fmt.Fprintf(os.Stderr, "rotate gemini failed: %v\n", err)
				os.Exit(1)
			}

			logger.Printf(
				"DEBUG cli complete command=%s previous_email=%s selected_email=%s account_count=%d",
				command,
				result.PreviousEmail,
				result.SelectedEmail,
				result.AccountCount,
			)

			fmt.Printf("rotated gemini account: %s -> %s\n", result.PreviousEmail, result.SelectedEmail)

		default:
			logger.Printf("DEBUG cli error command=%s unknown_provider=%s", command, *provider)
			fmt.Fprintf(os.Stderr, "unknown provider: %s (use openai or gemini)\n", *provider)
			os.Exit(1)
		}

	case "sync":
		switch *provider {
		case "openai":
			result, err := service.SyncOpenAIAndCodex(*configPath, *openAITargetPath, *codexTargetPath)
			if err != nil {
				logger.Printf("DEBUG cli error command=%s config=%s target=%s err=%v", command, *configPath, *openAITargetPath, err)
				fmt.Fprintf(os.Stderr, "sync openai and codex failed: %v\n", err)
				os.Exit(1)
			}

			logger.Printf(
				"DEBUG cli complete command=%s selected_email=%s account_count=%d",
				command,
				result.SelectedEmail,
				result.AccountCount,
			)

			fmt.Printf("synced openai and codex account: %s\n", result.SelectedEmail)

		case "gemini":
			result, err := service.SyncGemini(*configPath, *geminiActivePath)
			if err != nil {
				logger.Printf("DEBUG cli error command=%s config=%s target=%s err=%v", command, *configPath, *geminiActivePath, err)
				fmt.Fprintf(os.Stderr, "sync gemini failed: %v\n", err)
				os.Exit(1)
			}

			logger.Printf(
				"DEBUG cli complete command=%s selected_email=%s account_count=%d",
				command,
				result.SelectedEmail,
				result.AccountCount,
			)

			fmt.Printf("synced gemini account: %s\n", result.SelectedEmail)

		default:
			logger.Printf("DEBUG cli error command=%s unknown_provider=%s", command, *provider)
			fmt.Fprintf(os.Stderr, "unknown provider: %s (use openai or gemini)\n", *provider)
			os.Exit(1)
		}

	case "import":
		switch *provider {
		case "opencode":
			result, err := service.ImportOpenCode(*configPath, *openAITargetPath)
			if err != nil {
				logger.Printf("DEBUG cli error command=%s config=%s target=%s err=%v", command, *configPath, *openAITargetPath, err)
				fmt.Fprintf(os.Stderr, "import opencode failed: %v\n", err)
				os.Exit(1)
			}

			logger.Printf(
				"DEBUG cli complete command=%s previous_email=%s selected_email=%s account_count=%d",
				command,
				result.PreviousEmail,
				result.SelectedEmail,
				result.AccountCount,
			)

			fmt.Printf("imported opencode account into central credentials: %s -> %s\n", result.PreviousEmail, result.SelectedEmail)

		case "codex":
			result, err := service.ImportCodex(*configPath, *codexTargetPath)
			if err != nil {
				logger.Printf("DEBUG cli error command=%s config=%s target=%s err=%v", command, *configPath, *codexTargetPath, err)
				fmt.Fprintf(os.Stderr, "import codex failed: %v\n", err)
				os.Exit(1)
			}

			logger.Printf(
				"DEBUG cli complete command=%s previous_email=%s selected_email=%s account_count=%d",
				command,
				result.PreviousEmail,
				result.SelectedEmail,
				result.AccountCount,
			)

			fmt.Printf("imported codex account into central credentials: %s -> %s\n", result.PreviousEmail, result.SelectedEmail)

		default:
			logger.Printf("DEBUG cli error command=%s unknown_provider=%s", command, *provider)
			fmt.Fprintf(os.Stderr, "unknown provider: %s (use opencode or codex)\n", *provider)
			os.Exit(1)
		}

	default:
		logger.Printf("DEBUG cli error unknown_command=%s", command)
		fmt.Fprintf(os.Stderr, "unknown command: %s (use rotate, sync, or import)\n", command)
		os.Exit(1)
	}
}
