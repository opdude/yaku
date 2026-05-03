package translate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
}

// Stream translates text by calling onToken for each token as it streams.
// prevContext (may be empty) is the tail of previous translated output; it helps
// the model produce natural continuations across consecutive audio chunks.
func Stream(ollamaURL, model, text, srcLang, dstLang, prevContext string, onToken func(string)) error {
	prompt := buildPrompt(text, srcLang, dstLang, prevContext)

	body, err := json.Marshal(ollamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: true,
	})
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := http.Post(ollamaURL, "application/json", bytes.NewBuffer(body)) //nolint:noctx
	if err != nil {
		return fmt.Errorf("Ollama unreachable — is it running? (ollama serve): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Try to read Ollama's JSON error body for a helpful message.
		var errBody struct {
			Error string `json:"error"`
		}
		if resp.StatusCode == http.StatusNotFound {
			_ = json.NewDecoder(resp.Body).Decode(&errBody)
			msg := errBody.Error
			if msg == "" {
				msg = fmt.Sprintf("model %q not found", model)
			}
			return fmt.Errorf("Ollama model not found — run: ollama pull %s (%s)", model, msg)
		}
		return fmt.Errorf("Ollama returned HTTP %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r ollamaResponse
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		if r.Error != "" {
			return fmt.Errorf("Ollama error: %s", r.Error)
		}
		if onToken != nil && r.Response != "" {
			onToken(r.Response)
		}
		if r.Done {
			break
		}
	}
	return scanner.Err()
}

func buildPrompt(text, srcLang, dstLang, prevContext string) string {
	src := LangName(srcLang)
	dst := LangName(dstLang)

	if prevContext != "" {
		return fmt.Sprintf(
			"Translate from %s to %s. Use the previous subtitle only for context on names and tone.\n"+
				"Previous subtitle (context only, do NOT repeat it): \"%s\"\n\n"+
				"Translate ONLY the new text below. Output the %s translation only, nothing else, if you don't know how to translate, just return the original text:\n%s",
			src, dst, prevContext, dst, text,
		)
	}
	return fmt.Sprintf(
		"Translate the following %s text to %s. Output only the translation, nothing else, if you don't know how to translate, just return the original text:\n\n%s",
		src, dst, text,
	)
}

// LangName converts a BCP-47 code to a human-readable name for use in prompts.
// Unknown codes are returned as-is.
func LangName(code string) string {
	names := map[string]string{
		"de": "German", "en": "English", "fr": "French",
		"es": "Spanish", "it": "Italian", "pt": "Portuguese",
		"nl": "Dutch", "pl": "Polish", "ru": "Russian",
		"ja": "Japanese", "zh": "Chinese", "ko": "Korean",
		"ar": "Arabic", "tr": "Turkish", "sv": "Swedish",
		"da": "Danish", "no": "Norwegian", "fi": "Finnish",
		"cs": "Czech", "hu": "Hungarian", "ro": "Romanian",
		"auto": "the detected language",
	}
	if n, ok := names[strings.ToLower(code)]; ok {
		return n
	}
	return code
}
