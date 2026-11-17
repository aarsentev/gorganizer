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

	"github.com/openai/openai-go/v3/option"
)

// fakeAPI answers POST /v1/responses with the given status and output, and keeps
// the last request so the test can look at what was sent.
type fakeAPI struct {
	header http.Header
	body   map[string]any
}

func (f *fakeAPI) parser(t *testing.T, status string, output ...map[string]any) *Parser {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		f.header = r.Header.Clone()
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &f.body); err != nil {
			t.Errorf("request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		response := map[string]any{
			"id": "resp_1", "object": "response", "model": "gpt-5-mini",
			"status": status, "output": output,
		}
		if status == "incomplete" {
			response["incomplete_details"] = map[string]any{"reason": "max_output_tokens"}
		}
		json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(srv.Close)
	p, err := NewParser("gpt-5-mini", option.WithAPIKey("test-key"), option.WithBaseURL(srv.URL+"/v1/"), option.WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func message(content ...map[string]any) map[string]any {
	return map[string]any{"type": "message", "id": "msg_1", "role": "assistant", "status": "completed", "content": content}
}

func text(s string) map[string]any {
	return message(map[string]any{"type": "output_text", "text": s, "annotations": []any{}})
}

var thinking = map[string]any{"type": "reasoning", "id": "rs_1", "summary": []any{}}

var tuesday = time.Date(2026, 9, 29, 10, 0, 0, 0, time.FixedZone("CEST", 2*60*60))

const seminar = `{"action": "create", "kind": "event", "title": "Семинар", "start": "2026-09-30T10:00:00+02:00", "when": "", "duration_min": 90, "location": "", "text": ""}`

func TestParse(t *testing.T) {
	var api fakeAPI
	p := api.parser(t, "completed", thinking, text(seminar))

	in, err := p.Parse(context.Background(), Input{Text: "семинар завтра с 10 до 11:30", Now: tuesday})
	if err != nil {
		t.Fatal(err)
	}
	if in.Action != "create" || in.Kind != "event" || in.Title != "Семинар" || in.DurationMin != 90 ||
		in.Start == nil || !in.Start.Equal(tuesday.Add(24*time.Hour)) {
		t.Fatalf("intent = %+v", in)
	}

	// Check the request sent to the Responses API.
	body := api.body
	if body["model"] != "gpt-5-mini" || api.header.Get("Authorization") != "Bearer test-key" {
		t.Errorf("model %v, authorization %q", body["model"], api.header.Get("Authorization"))
	}
	format := body["text"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["name"] != "intent" || format["strict"] != true ||
		format["schema"].(map[string]any)["required"] == nil {
		t.Errorf("format = %v", format)
	}
	if body["reasoning"].(map[string]any)["effort"] != "medium" || body["store"] != false ||
		body["max_output_tokens"] != float64(maxTokens) {
		t.Errorf("reasoning = %v, store = %v, max_output_tokens = %v", body["reasoning"], body["store"], body["max_output_tokens"])
	}
	if system, _ := body["instructions"].(string); !strings.Contains(system, "Never guess duration, location or time.") {
		t.Errorf("instructions = %.200s…", system)
	}
	message := body["input"]
	if message != "NOW: 2026-09-29T10:00:00+02:00 (Tuesday)\nMessage: семинар завтра с 10 до 11:30" {
		t.Errorf("message = %q", message)
	}
	// The request uses structured output without sampling overrides or tools.
	for _, key := range []string{"temperature", "top_p", "tools", "tool_choice"} {
		if _, ok := body[key]; ok {
			t.Errorf("request sends %s", key)
		}
	}
}

func TestParseFailures(t *testing.T) {
	cases := []struct {
		name   string
		status string
		output []map[string]any
		want   error // nil: some error other than ErrRefused
	}{
		{"refusal", "completed", []map[string]any{message(map[string]any{"type": "refusal", "refusal": "I cannot help with that."})}, ErrRefused},
		{"cut at max_output_tokens", "incomplete", []map[string]any{text(seminar)}, nil},
		{"no text", "completed", []map[string]any{thinking}, nil},
		{"a field Intent does not know", "completed", []map[string]any{text(`{"action": "create", "rrule": "FREQ=WEEKLY"}`)}, nil},
		{"not JSON", "completed", []map[string]any{text("Sure! Here is the intent.")}, nil},
	}
	for _, c := range cases {
		var api fakeAPI
		p := api.parser(t, c.status, c.output...)
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
		w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"model is not supported"}}`))
	}))
	t.Cleanup(srv.Close)
	p, err := NewParser("gpt-5-mini", option.WithAPIKey("test-key"), option.WithBaseURL(srv.URL+"/v1/"), option.WithMaxRetries(0))
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
