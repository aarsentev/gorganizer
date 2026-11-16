package intent

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type schema struct {
	AdditionalProperties bool                       `json:"additionalProperties"`
	Required             []string                   `json:"required"`
	Properties           map[string]json.RawMessage `json:"properties"`
}

func loadSchema(t *testing.T) schema {
	t.Helper()
	var s schema
	if err := json.Unmarshal([]byte(Schema), &s); err != nil {
		t.Fatalf("schema.json: %v", err)
	}
	return s
}

// jsonNames are the JSON names of Intent's fields, the contract with drafts and the model.
func jsonNames() []string {
	var names []string
	for field := range reflect.TypeFor[Intent]().Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		names = append(names, name)
	}
	return names
}

func sameSet(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

// The schema and Intent change together: a field renamed in Go alone would leave the
// model answering in a form the app no longer reads.
func TestSchemaMatchesIntent(t *testing.T) {
	s := loadSchema(t)
	var properties []string
	for name := range s.Properties {
		properties = append(properties, name)
	}
	if !sameSet(properties, jsonNames()) {
		t.Fatalf("schema properties %v, Intent fields %v", properties, jsonNames())
	}
	// Structured outputs need a closed object; every field is required, empty when not said.
	if s.AdditionalProperties || !sameSet(s.Required, properties) {
		t.Fatalf("additionalProperties = %v, required = %v; want false and all of %v", s.AdditionalProperties, s.Required, properties)
	}
}

func TestExamples(t *testing.T) {
	var examples []example
	if err := yaml.Unmarshal([]byte(Examples), &examples); err != nil {
		t.Fatalf("examples.yaml: %v", err)
	}
	if len(examples) < 10 {
		t.Fatalf("%d examples, want at least 10", len(examples))
	}
	properties := loadSchema(t).Required
	for _, e := range examples {
		// Every field is there, as the schema makes the model answer.
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(e.Answer), &fields); err != nil {
			t.Errorf("%q: answer is not JSON: %v", e.Message, err)
			continue
		}
		var keys []string
		for key := range fields {
			keys = append(keys, key)
		}
		if !sameSet(keys, properties) {
			t.Errorf("%q: answer has %v, want all of %v", e.Message, keys, properties)
		}

		// It reads into Intent with nothing left over, and passes Validate at its NOW.
		in, err := decodeAnswer(e.Answer)
		if err != nil {
			t.Errorf("%q: %v", e.Message, err)
			continue
		}
		if e.Now.IsZero() || e.Message == "" {
			t.Errorf("example %+v has no now or no message", e)
		}
		err = in.Validate(e.Now)
		if err != nil && !(errors.Is(err, ErrUnsupported) && (in.Action == "update" || in.Action == "delete")) {
			t.Errorf("%q: Validate() = %v", e.Message, err)
		}
	}
}

func TestPromptVersion(t *testing.T) {
	version := PromptVersion()
	if len(version) != 8 || strings.Trim(version, "0123456789abcdef") != "" {
		t.Fatalf("PromptVersion() = %q, want 8 hex digits", version)
	}
	saved := System
	System += " "
	changed := PromptVersion()
	System = saved
	if changed == version {
		t.Fatal("a change to the prompt kept the version")
	}
}
