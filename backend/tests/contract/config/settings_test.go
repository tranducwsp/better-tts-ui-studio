package config_test

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"backend/config"
	"backend/tests/testsupport"
)

func envExamplePath(t *testing.T) string {
	t.Helper()
	return testsupport.Path(t, ".env.example")
}

func backendDir(t *testing.T) string {
	t.Helper()
	return testsupport.Path(t, "backend")
}

func configSourcePath(t *testing.T) string {
	t.Helper()
	return testsupport.Path(t, "backend", "config", "config.go")
}

// TestEnvExampleIsUpToDate runs the generator into a temp file and compares it with the committed file.
//
// Without this test, forgetting `go generate` after editing config.Settings would leave .env.example describing
// a configuration the backend no longer uses — exactly the kind of silent drift that generated specs are
// meant to eliminate.
func TestEnvExampleIsUpToDate(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), ".env.example")

	cmd := exec.Command("go", "run", "./cmd/gen-env")
	cmd.Dir = backendDir(t)
	cmd.Env = append(os.Environ(), "GEN_ENV_OUT="+tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gen-env failed: %v\n%s", err, out)
	}

	want, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	got, err := os.ReadFile(envExamplePath(t))
	if err != nil {
		t.Fatalf("reading committed .env.example: %v", err)
	}

	if string(got) != string(want) {
		t.Error(".env.example has drifted from config/settings.go.\n" +
			"Run: cd backend && go generate ./config")
	}
}

// TestEnvExampleCoversEverySetting reads the committed file as an operator would, and
// checks that no variable is missing or extra.
func TestEnvExampleCoversEverySetting(t *testing.T) {
	f, err := os.Open(envExamplePath(t))
	if err != nil {
		t.Fatalf("opening .env.example: %v", err)
	}
	defer f.Close()

	inFile := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, _, ok := strings.Cut(line, "="); ok {
			inFile[strings.TrimSpace(k)] = true
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading .env.example: %v", err)
	}

	inSpec := map[string]bool{}
	for _, s := range config.Settings {
		inSpec[s.Key] = true
		if !inFile[s.Key] {
			t.Errorf("%s is in config.Settings but missing from .env.example", s.Key)
		}
	}
	for k := range inFile {
		if !inSpec[k] {
			t.Errorf("%s is in .env.example but not in config.Settings", k)
		}
	}
}

// TestSecretsNeverCarryAValue protects against an easy accident: a real key being filled into the sample file
// and committed.
func TestSecretsNeverCarryAValue(t *testing.T) {
	raw, err := os.ReadFile(envExamplePath(t))
	if err != nil {
		t.Fatalf("reading .env.example: %v", err)
	}
	text := string(raw)

	for _, s := range config.Settings {
		if s.Kind != config.KindSecret {
			continue
		}
		if !strings.Contains(text, s.Key+"=\n") && !strings.HasSuffix(text, s.Key+"=") {
			t.Errorf("%s is a secret so it must be left empty in .env.example", s.Key)
		}
	}
}

// TestIntSettingsHaveUsableRanges catches declaration errors: default outside the declared range,
// or min greater than max. num() would log.Fatalf at runtime, exposing it only at deploy time;
// this test catches it at build time.
func TestIntSettingsHaveUsableRanges(t *testing.T) {
	for _, s := range config.Settings {
		if s.Kind != config.KindInt {
			continue
		}
		if s.Min > s.Max {
			t.Errorf("%s: Min (%d) is greater than Max (%d)", s.Key, s.Min, s.Max)
		}
		v, err := strconv.Atoi(s.Default)
		if err != nil {
			t.Errorf("%s: default %q is not an integer", s.Key, s.Default)
			continue
		}
		if v < s.Min || v > s.Max {
			t.Errorf("%s: default %d is outside the declared range (%d to %d)", s.Key, v, s.Min, s.Max)
		}
	}
}

// TestNoDuplicateKeys catches a key being declared twice, where lookup() silently returns
// the first one and the second never takes effect.
func TestNoDuplicateKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range config.Settings {
		if seen[s.Key] {
			t.Errorf("key %s is declared multiple times in config.Settings", s.Key)
		}
		seen[s.Key] = true
	}
}

// TestReadByVarsAreNotLoaded maintains the boundary: a variable marked with ReadBy belongs to another
// service, so LoadConfig must not read it. If the backend later needs it, remove ReadBy
// instead of letting the spec say one thing and the source do another.
func TestReadByVarsAreNotLoaded(t *testing.T) {
	raw, err := os.ReadFile(configSourcePath(t))
	if err != nil {
		t.Fatalf("reading config.go: %v", err)
	}
	src := string(raw)

	for _, s := range config.Settings {
		if s.ReadBy == "" {
			continue
		}
		for _, call := range []string{`str("` + s.Key + `")`, `num("` + s.Key + `")`} {
			if strings.Contains(src, call) {
				t.Errorf("%s is marked ReadBy=%q but LoadConfig still calls %s", s.Key, s.ReadBy, call)
			}
		}
	}
}

// TestRequiredSettingsAreEnforced confirms the Required flag actually blocks startup.
//
// Previously it was purely decorative: gen-env read it to print "REQUIRED" in .env.example, but at
// runtime no one checked — so a deployment missing SECRET_KEY would still start with a random key,
// and operators would only know if they happened to read the logs.
//
// requireAll calls log.Fatalf, so it cannot be called directly in a test; instead, verify the
// set it iterates over, and confirm it is called from LoadConfig.
func TestRequiredSettingsAreEnforced(t *testing.T) {
	raw, err := os.ReadFile(configSourcePath(t))
	if err != nil {
		t.Fatalf("reading config.go: %v", err)
	}
	src := string(raw)

	if !strings.Contains(src, "requireAll()") {
		t.Error("LoadConfig must call requireAll(), otherwise the Required flag is just a comment")
	}

	// At least one variable must be marked Required, otherwise the test above is meaningless.
	found := false
	for _, s := range config.Settings {
		if s.Required {
			found = true
			break
		}
	}
	if !found {
		t.Error("no variable is Required — check the config.Settings table")
	}
}

// TestRequiredBackendVarsHaveNoDefault: a variable being both Required and having a Default is contradictory.
// requireAll only checks ENV, so Default would never be used and only causes confusion when reading the table.
func TestRequiredBackendVarsHaveNoDefault(t *testing.T) {
	for _, s := range config.Settings {
		if s.Required && s.Default != "" {
			t.Errorf("%s is both Required and has a Default %q — drop one of the two", s.Key, s.Default)
		}
	}
}
