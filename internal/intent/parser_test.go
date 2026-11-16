package intent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// fakeAPI answers POST /v1/messages with the given content and stop reason, and keeps
// the last request so the test can look at what was sent.
type fakeAPI struct {
	header http.Header
	body   map[string]any
}

func (f *fakeAPI) parser(t *testing.T, stopReason string, content ...map[string]any) *Parser {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.header = r.Header.Clone()
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &f.body); err != nil {
			t.Errorf("request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-haiku-5-5",
			"content": content, "stop_reason": stopReason, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	t.Cleanup(srv.Close)
	p, err := NewParser("claude-haiku-5-5", option.WithAPIKey("test-key"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func text(s string) map[string]any { return map[string]any{"type": "text", "text": s} }

// thinking is how Haiku 5.5 sends its reasoning by default: the text is omitted.
var thinking = map[string]any{"type": "thinking", "thinking": "", "signature": "sig"}

var tuesday = time.Date(2026, 9, 29, 10, 0, 0, 0, time.FixedZone("CEST", 2*60*60))

const seminar = `{"action": "create", "kind": "event", "title": "Семинар", "start": "2026-09-30T10:00:00+02:00", "when": "", "duration_min": 90, "location": "", "text": ""}`

func TestParse(t *testing.T) {
	var api fakeAPI
	p := api.parser(t, "end_turn", thinking, text(seminar))

	in, err := p.Parse(context.Background(), Input{Text: "семинар завтра с 10 до 11:30", Now: tuesday})
	if err != nil {
		t.Fatal(err)
	}
	if in.Action != "create" || in.Kind != "event" || in.Title != "Семинар" || in.DurationMin != 90 ||
		in.Start == nil || !in.Start.Equal(tuesday.Add(24*time.Hour)) {
		t.Fatalf("intent = %+v", in)
	}

	// What went out: the model, the key, the effort, the schema, the cached prompt, the message.
	body := api.body
	if body["model"] != "claude-haiku-5-5" || api.header.Get("Authorization") != "Bearer test-key" {
		t.Errorf("model %v, authorization %q", body["model"], api.header.Get("Authorization"))
	}
	if version := api.header.Get("Anthropic-Version"); version != "2026-10-21" {
		t.Errorf("anthropic-version = %q", version) // the version on which temperature is a 400
	}
	config := body["output_config"].(map[string]any)
	format := config["format"].(map[string]any)
	if config["effort"] != "medium" || format["type"] != "json_schema" || format["schema"].(map[string]any)["required"] == nil {
		t.Errorf("output_config = %v", config)
	}
	system := body["system"].([]any)[0].(map[string]any)
	if !strings.Contains(system["text"].(string), "Never guess duration, location or time.") || system["cache_control"] == nil {
		t.Errorf("system = %.200v…", system)
	}
	message := body["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"]
	if message != "NOW: 2026-09-29T10:00:00+02:00 (Tuesday)\nMessage: семинар завтра с 10 до 11:30" {
		t.Errorf("message = %q", message)
	}
	// Haiku 5.5 rejects sampling parameters, and thinking is left to its default.
	for _, key := range []string{"temperature", "top_p", "top_k", "thinking", "tools", "tool_choice"} {
		if _, ok := body[key]; ok {
			t.Errorf("request sends %s", key)
		}
	}
}

func TestParseFailures(t *testing.T) {
	cases := []struct {
		name       string
		stopReason string
		content    []map[string]any
		want       error // nil: some error other than ErrRefused
	}{
		{"refusal", "refusal", nil, ErrRefused},
		{"cut at max_tokens", "max_tokens", []map[string]any{thinking}, nil},
		{"no text", "end_turn", []map[string]any{thinking}, nil},
		{"a field Intent does not know", "end_turn", []map[string]any{text(`{"action": "create", "rrule": "FREQ=WEEKLY"}`)}, nil},
		{"not JSON", "end_turn", []map[string]any{text("Sure! Here is the intent.")}, nil},
	}
	for _, c := range cases {
		var api fakeAPI
		p := api.parser(t, c.stopReason, c.content...)
		_, err := p.Parse(context.Background(), Input{Text: "x", Now: tuesday})
		if err == nil || (c.want != nil && !errors.Is(err, c.want)) || (c.want == nil && errors.Is(err, ErrRefused)) {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

func TestParseAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"temperature is not supported"}}`))
	}))
	t.Cleanup(srv.Close)
	p, err := NewParser("claude-haiku-5-5", option.WithAPIKey("test-key"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Parse(context.Background(), Input{Text: "x", Now: tuesday}); err == nil {
		t.Fatal("a 400 from the API must be an error")
	}
}

// The examples reach the model in the same form as a real message, with their own NOW.
func TestSystemPrompt(t *testing.T) {
	system, err := systemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	want := "NOW: 2026-09-29T10:00:00+02:00 (Tuesday)\nMessage: семинар завтра с 10 до 11:30\nAnswer: " + seminar + "\n"
	if !strings.HasPrefix(system, System) || !strings.Contains(system, want) {
		t.Fatalf("system prompt lacks the example in message form:\n%s", system[len(System):])
	}
}
