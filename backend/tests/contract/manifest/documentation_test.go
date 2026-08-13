package manifest_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"backend/types"
)

// The full Manifest published in docs/GATEWAY.md must pass a clean validation — no errors
// and no warnings. Read the JSON block directly from Markdown so the doc is the single source; a
// separate JSON copy will inevitably drift from the example that AI Engineers actually read and copy.
//
// This is a two-way fence. If someone adds an overly strict validation rule, this test breaks instead
// of letting operators see warnings on a healthy Engine and learn to ignore all warnings.
// If the doc example becomes stale or is no longer a valid manifest, CI also catches it immediately.
func TestDocumentedManifestPassesValidation(t *testing.T) {
	const path = "../../../../docs/GATEWAY.md"
	const heading = "## 💻 3. COMPLETE STANDARD MANIFEST EXAMPLE"

	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}

	sectionAt := bytes.Index(doc, []byte(heading))
	if sectionAt < 0 {
		t.Fatalf("%s no longer contains section %q", path, heading)
	}
	section := doc[sectionAt:]

	const openFence = "```json\n"
	openAt := bytes.Index(section, []byte(openFence))
	if openAt < 0 {
		t.Fatalf("manifest section in %s has no ```json block", path)
	}
	payload := section[openAt+len(openFence):]

	closeAt := bytes.Index(payload, []byte("\n```"))
	if closeAt < 0 {
		t.Fatalf("manifest block in %s has no closing fence", path)
	}

	var m types.UniversalManifest
	if err := json.Unmarshal(payload[:closeAt], &m); err != nil {
		t.Fatalf("manifest block in %s is not valid JSON: %v", path, err)
	}

	errs, warnings := m.Validate()
	for _, w := range warnings {
		t.Logf("warning: %s", w)
	}
	if len(errs) > 0 {
		t.Errorf("manifest in documentation was rejected: %v", errs)
	}
	if len(warnings) > 0 {
		t.Errorf("manifest in documentation produced %d warnings; either the example is stale or the validation rule is too strict", len(warnings))
	}
}
