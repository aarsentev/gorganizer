package intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
	"gopkg.in/yaml.v3"
)

// ErrRefused means the model declined the message for safety reasons.
var ErrRefused = errors.New("model refused")

const (
	// maxTokens leaves room for thinking, which counts toward it: the answer is ~100 tokens.
	maxTokens = 2048
	// requestTimeout keeps a hanging API from holding the chat; the SDK retries twice on top.
	requestTimeout = 20 * time.Second
)

// Input is the message together with what the parser cannot know on its own.
type Input struct {
	Text string
	Now  time.Time // in the user's zone; passed in, not read from the clock, so eval repeats
}

// Parser turns a message into an Intent through the OpenAI Responses API.
type Parser struct {
	client openai.Client
	model  string
	system string
	schema map[string]any
}

// NewParser builds a parser for the model. opts go to the API client: the key, and the
// address of a fake server in tests.
func NewParser(model string, opts ...option.RequestOption) (*Parser, error) {
	system, err := systemPrompt()
	if err != nil {
		return nil, err
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(Schema), &schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	opts = append([]option.RequestOption{option.WithRequestTimeout(requestTimeout)}, opts...)
	return &Parser{client: openai.NewClient(opts...), model: model, system: system, schema: schema}, nil
}

// systemPrompt is the instructions followed by the examples: the same on every call, so
// the API can cache it.
func systemPrompt() (string, error) {
	var examples []example
	if err := yaml.Unmarshal([]byte(Examples), &examples); err != nil {
		return "", fmt.Errorf("examples: %w", err)
	}
	var b strings.Builder
	b.WriteString(System)
	b.WriteString("\nExamples:\n")
	for _, e := range examples {
		fmt.Fprintf(&b, "\n%s\nAnswer: %s\n", userText(Input{Text: e.Message, Now: e.Now}), e.Answer)
	}
	return b.String(), nil
}

// userText is what the model gets for a message: NOW with its weekday, so that "on Friday"
// counts from the right day, then the message itself.
func userText(in Input) string {
	return fmt.Sprintf("NOW: %s (%s)\nMessage: %s", in.Now.Format(time.RFC3339), in.Now.Weekday(), in.Text)
}

// Parse asks the model what the message means. The answer is not validated here: the app
// checks it with Validate before acting on it.
func (p *Parser) Parse(ctx context.Context, in Input) (Intent, error) {
	resp, err := p.client.Responses.New(ctx, responses.ResponseNewParams{
		Model:           p.model,
		Instructions:    openai.String(p.system),
		Input:           responses.ResponseNewParamsInputUnion{OfString: openai.String(userText(in))},
		MaxOutputTokens: openai.Int(maxTokens),
		Reasoning:       shared.ReasoningParam{Effort: shared.ReasoningEffortMedium},
		Store:           openai.Bool(false),
		Text: responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
					Name:   "intent",
					Schema: p.schema,
					Strict: openai.Bool(true),
				},
			},
		},
	})
	if err != nil {
		return Intent{}, fmt.Errorf("parse: %w", err)
	}
	for _, item := range resp.Output {
		if item.Type != "message" {
			continue
		}
		for _, part := range item.AsMessage().Content {
			if part.Type == "refusal" {
				return Intent{}, ErrRefused
			}
		}
	}
	if resp.Status == "incomplete" {
		return Intent{}, fmt.Errorf("parse: incomplete response (%s)", resp.IncompleteDetails.Reason)
	}
	if resp.Status != "completed" {
		return Intent{}, fmt.Errorf("parse: response status %q", resp.Status)
	}
	answer := resp.OutputText()
	if answer == "" {
		return Intent{}, errors.New("parse: no text in the answer")
	}
	return decodeAnswer(answer)
}

// decodeAnswer reads the model's JSON. A field Intent does not know is an error: the schema
// and Intent have drifted apart.
func decodeAnswer(answer string) (Intent, error) {
	var in Intent
	decoder := json.NewDecoder(strings.NewReader(answer))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		return Intent{}, fmt.Errorf("parse: answer %q: %w", answer, err)
	}
	return in, nil
}
