package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const exampleSchemaPath = "../../core-tts-example/schemas.py"

// TestExampleDeclaresVoiceModes checks that VoiceInfo.modes is a required field.
//
// If someone adds back `= Field(default_factory=list)`, Pydantic accepts a voice with no
// declared modes, and the platform's filtering will silently return an empty list.
func TestExampleDeclaresVoiceModes(t *testing.T) {
	src := readSchema(t, exampleSchemaPath)

	block, ok := classBlock(src, "VoiceInfo")
	if !ok {
		t.Fatalf("%s: class VoiceInfo not found", exampleSchemaPath)
	}

	line := ""
	for _, l := range strings.Split(block, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "modes:") {
			line = strings.TrimSpace(l)
			break
		}
	}
	if line == "" {
		t.Errorf("%s: VoiceInfo must declare `modes`", filepath.Base(exampleSchemaPath))
		return
	}
	if strings.Contains(line, "=") {
		t.Errorf("%s: `modes` must be a required field, no default — currently %q",
			filepath.Base(exampleSchemaPath), line)
	}
}

// TestExampleSchemaParses checks that the example engine's schemas.py is readable.
//
// The real engine is no longer in the repo — AI engineers replace it with their own.
// The example engine is the contract reference and must be valid.
func TestExampleSchemaParses(t *testing.T) {
	classes := parsePydanticClasses(t, exampleSchemaPath)
	if len(classes) == 0 {
		t.Fatalf("no classes read from %s", exampleSchemaPath)
	}

	// Classes that are required in the example engine
	required := []string{"SynthesizeRequest", "VoiceInfo", "UniversalManifest"}
	for _, name := range required {
		if _, ok := classes[name]; !ok {
			t.Errorf("example is missing class %s", name)
		}
	}

	// SynthesizeRequest must have the fields the platform sends
	srFields, ok := classes["SynthesizeRequest"]
	if !ok {
		return
	}
	for _, f := range []string{"text", "voice_id", "speed", "engine"} {
		if _, ok := srFields[f]; !ok {
			t.Errorf("SynthesizeRequest missing field %q", f)
		}
	}
}

var (
	classRe = regexp.MustCompile(`(?m)^class\s+(\w+)\s*\(`)
	fieldRe = regexp.MustCompile(`(?m)^\s{4}(\w+)\s*:\s*\S`)
)

func readSchema(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func classBlock(src, name string) (string, bool) {
	locs := classRe.FindAllStringSubmatchIndex(src, -1)
	for i, loc := range locs {
		if src[loc[2]:loc[3]] != name {
			continue
		}
		end := len(src)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		return src[loc[0]:end], true
	}
	return "", false
}

func parsePydanticClasses(t *testing.T, path string) map[string]map[string]struct{} {
	t.Helper()
	src := readSchema(t, path)

	out := map[string]map[string]struct{}{}
	locs := classRe.FindAllStringSubmatchIndex(src, -1)
	for i, loc := range locs {
		name := src[loc[2]:loc[3]]
		end := len(src)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}

		fields := map[string]struct{}{}
		for _, m := range fieldRe.FindAllStringSubmatch(src[loc[1]:end], -1) {
			fields[m[1]] = struct{}{}
		}
		out[name] = fields
	}
	return out
}
