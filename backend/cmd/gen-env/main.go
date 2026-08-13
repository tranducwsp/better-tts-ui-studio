// Command gen-env generates .env.example from the spec table in config/settings.go.
//
// Run via `go generate ./config` after adding or modifying a variable. The generated file is
// committed so operators can read it without Go tooling, and config.TestEnvExampleMatchesSpec
// compares the committed file against the table, so forgetting to run generate will make the
// test fail.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"backend/config"
)

const header = `# Auto-generated from backend/config/settings.go — do not edit by hand.
# Update with: cd backend && go generate ./config
#
# Copy to .env and fill in before deploying. Displayed values are the backend defaults
# used when the variable is not set; variables marked REQUIRED have no safe default.
# The "Advanced deployment tuning" group is optional for operators; AI engineers usually
# don't need to change them.
`

func main() {
	// go generate runs with cwd at the directory containing the directive (config/), so go up
	// two levels to reach the repo root. Allow override for manual runs from elsewhere.
	out := os.Getenv("GEN_ENV_OUT")
	if out == "" {
		out = filepath.Join("..", "..", ".env.example")
	}
	out, err := filepath.Abs(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-env:", err)
		os.Exit(1)
	}

	var b strings.Builder
	b.WriteString(header)

	lastGroup := ""
	for _, s := range config.Settings {
		if s.Group != lastGroup {
			b.WriteString("\n" + sectionRule(s.Group) + "\n")
			lastGroup = s.Group
		}

		if s.Required {
			b.WriteString("# REQUIRED for production environments.\n")
		}
		if s.ReadBy != "" {
			fmt.Fprintf(&b, "# Read by %s, not the backend.\n", s.ReadBy)
		}
		for _, line := range docLines(s.Doc) {
			b.WriteString("# " + line + "\n")
		}
		if s.Kind == config.KindInt {
			fmt.Fprintf(&b, "# Integer in range %d to %d.\n", s.Min, s.Max)
		}

		// Secrets must never have their value written into the example file, even when the
		// default is empty — so no one accidentally commits a real value right here.
		value := s.Default
		if s.Kind == config.KindSecret {
			value = ""
		}
		fmt.Fprintf(&b, "%s=%s\n", s.Key, value)
	}

	if err := os.WriteFile(out, []byte(b.String()), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "gen-env:", err)
		os.Exit(1)
	}
	fmt.Printf("gen-env: wrote %s (%d variables)\n", out, len(config.Settings))
}

// sectionRule creates a separator padded to 90 columns for readability.
func sectionRule(group string) string {
	const width = 90
	prefix := "# ── " + group + " "
	if n := width - len([]rune(prefix)); n > 0 {
		return prefix + strings.Repeat("─", n)
	}
	return prefix
}

func docLines(doc string) []string {
	if strings.TrimSpace(doc) == "" {
		return nil
	}
	return strings.Split(doc, "\n")
}
