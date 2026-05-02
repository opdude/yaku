package translate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockOllamaServer returns a test server that streams the given tokens as NDJSON.
func mockOllamaServer(t *testing.T, tokens []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		for i, tok := range tokens {
			done := i == len(tokens)-1
			resp := ollamaResponse{Response: tok, Done: done}
			line, _ := json.Marshal(resp)
			w.Write(line)   //nolint:errcheck
			w.Write([]byte("\n")) //nolint:errcheck
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
}

func TestStream_callsOnToken(t *testing.T) {
	tokens := []string{"The", " ", "cat", " ", "sat"}
	srv := mockOllamaServer(t, tokens)
	defer srv.Close()

	var received []string
	err := Stream(srv.URL, "test-model", "Die Katze saß", "de", "en", "", func(tok string) {
		received = append(received, tok)
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if len(received) != len(tokens) {
		t.Errorf("expected %d tokens, got %d: %v", len(tokens), len(received), received)
	}
}

func TestStream_prevContextPassedThrough(t *testing.T) {
	srv := mockOllamaServer(t, []string{"Good", " morning"})
	defer srv.Close()

	// Verify that a non-empty prevContext doesn't cause an error.
	var got strings.Builder
	err := Stream(srv.URL, "test-model", "Guten Morgen", "de", "en", "Hello.", func(tok string) {
		got.WriteString(tok)
	})
	if err != nil {
		t.Fatalf("Stream with prevContext: %v", err)
	}
	if got.Len() == 0 {
		t.Error("expected non-empty translation with prevContext")
	}
}

func TestStream_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := Stream(srv.URL, "test-model", "test", "de", "en", "", nil)
	if err == nil {
		t.Error("expected error for HTTP 500 response")
	}
}

func TestStream_unreachable(t *testing.T) {
	err := Stream("http://127.0.0.1:19999/api/generate", "model", "text", "de", "en", "", nil)
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestBuildPrompt_noContext(t *testing.T) {
	p := buildPrompt("Hallo Welt", "de", "en", "")
	if !strings.Contains(p, "German") {
		t.Errorf("prompt should mention source language, got: %q", p)
	}
	if !strings.Contains(p, "English") {
		t.Errorf("prompt should mention target language, got: %q", p)
	}
	if !strings.Contains(p, "Hallo Welt") {
		t.Errorf("prompt should contain the source text, got: %q", p)
	}
}

func TestBuildPrompt_withContext(t *testing.T) {
	p := buildPrompt("Wie geht es dir?", "de", "en", "Hello, my name is John.")
	if !strings.Contains(p, "Hello, my name is John.") {
		t.Errorf("prompt should include previous context, got: %q", p)
	}
	if !strings.Contains(p, "Wie geht es dir?") {
		t.Errorf("prompt should contain the source text, got: %q", p)
	}
	// Context-mode prompt should tell the model not to repeat the context.
	if !strings.Contains(p, "do NOT repeat") {
		t.Errorf("prompt should instruct model not to repeat context, got: %q", p)
	}
}

func TestLangName_knownCodes(t *testing.T) {
	cases := map[string]string{
		"de":   "German",
		"en":   "English",
		"fr":   "French",
		"auto": "the detected language",
	}
	for code, want := range cases {
		if got := LangName(code); got != want {
			t.Errorf("LangName(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestLangName_unknownCodePassthrough(t *testing.T) {
	// Unknown codes should be returned verbatim so the LLM still sees something.
	if got := LangName("xyz"); got != "xyz" {
		t.Errorf("LangName(unknown) = %q, want %q", got, "xyz")
	}
}

func TestLangName_caseInsensitive(t *testing.T) {
	if got := LangName("DE"); got != "German" {
		t.Errorf("LangName should be case-insensitive, got %q", got)
	}
}
