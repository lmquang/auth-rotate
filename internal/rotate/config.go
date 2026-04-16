package rotate

import "encoding/json"

type Credentials struct {
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
