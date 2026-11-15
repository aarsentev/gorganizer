package intent

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

// The prompt lives in files next to the code: prompts/ is all the model is told, and
// examples_test.go keeps it in step with Intent.

// System is the instructions, the same for every message, so the API can cache it.
//
//go:embed prompts/system.md
var System string

// Examples are answers the model is shown; in YAML with now, message and answer.
//
//go:embed prompts/examples.yaml
var Examples string

// Schema is the form of the answer, enforced by the API (structured outputs).
//
//go:embed prompts/schema.json
var Schema string

// PromptVersion is the first 8 hex digits of sha256 over the prompt files: llm_log keeps
// it with every call, so a worse answer can be traced to the change that caused it (SPEC §7).
func PromptVersion() string {
	sum := sha256.Sum256([]byte(System + "\x00" + Examples + "\x00" + Schema))
	return hex.EncodeToString(sum[:])[:8]
}
