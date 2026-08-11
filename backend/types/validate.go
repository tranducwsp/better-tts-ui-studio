package types

import (
	"fmt"
	"slices"
)

// Validate checks the Manifest for internal contradictions before it is accepted into RAM.
//
// This platform has already chosen fail-fast-at-startup elsewhere: a mistyped environment
// variable stops the process with a message naming the variable, because silent failures are
// much harder to fix. The Manifest, by contrast, was previously accepted unconditionally — Set()
// just assigned the pointer — so an Engine declaring default_format="opus" with
// supported_formats=["wav"] would still run, then emit files with a Content-Type no player can
// open, the declaration site separated from the symptom by tens of minutes of debugging.
//
// Returns (fatal errors, warnings). Split into two because the two kinds of mistakes differ
// greatly in consequence:
//
//   - Fatal errors: the resolver cannot produce a correct answer. No modes, or duplicate mode
//     IDs — everything downstream is guesswork, so reject the manifest.
//   - Warnings: resolvable, but the Engine almost certainly declared something wrong. Still
//     accept, and report, because rejecting an entire Manifest just because one ui_schema entry
//     points to a wrong name would crash a healthy Engine over a typo in the decoration layer.
func (m *UniversalManifest) Validate() (errs []error, warnings []string) {
	if m == nil {
		return []error{fmt.Errorf("manifest is empty")}, nil
	}

	if len(m.SupportedModes) == 0 {
		errs = append(errs, fmt.Errorf("manifest declares no modes in supported_modes"))
	}

	seen := make(map[string]struct{}, len(m.SupportedModes))
	for i, mode := range m.SupportedModes {
		if mode.ID == "" {
			errs = append(errs, fmt.Errorf("supported_modes[%d] has no id", i))
			continue
		}
		if _, dup := seen[mode.ID]; dup {
			// The resolver stops at the first matching mode, so the second copy is both invisible
			// and a sign the author thought they were configuring something else.
			errs = append(errs, fmt.Errorf("mode %q is duplicated in supported_modes", mode.ID))
			continue
		}
		seen[mode.ID] = struct{}{}
	}

	// Every check below reads the RESOLVED value, not what the Engine wrote: a mode that inherits
	// the Engine's default_format must still be consistent with the supported_formats it also
	// inherits, and only the resolver knows what that pair ultimately is.
	for _, mode := range m.SupportedModes {
		if mode.ID == "" {
			continue
		}
		spec := m.ResolveAudioSpec(mode.ID)
		caps := m.ResolveCapabilities(mode.ID)

		if len(spec.SupportedFormats) > 0 && !slices.Contains(spec.SupportedFormats, spec.DefaultFormat) {
			warnings = append(warnings, fmt.Sprintf(
				"mode %q: default_format %q is not in supported_formats %v — downloaded files will carry the Content-Type of a format the Engine did not declare it produces",
				mode.ID, spec.DefaultFormat, spec.SupportedFormats))
		}

		if len(spec.SupportedSampleRates) > 0 && !slices.Contains(spec.SupportedSampleRates, spec.DefaultSampleRate) {
			warnings = append(warnings, fmt.Sprintf(
				"mode %q: default_sample_rate %d is not in supported_sample_rates %v",
				mode.ID, spec.DefaultSampleRate, spec.SupportedSampleRates))
		}

		// Two ceilings exist for two moments: the raw file dragged in, and the clip after
		// trimming. If the latter ceiling exceeds the former the trimming step is meaningless.
		if spec.MaxUploadBytes > 0 && spec.MaxReferenceBytes > spec.MaxUploadBytes {
			warnings = append(warnings, fmt.Sprintf(
				"mode %q: max_reference_bytes (%d) exceeds max_upload_bytes (%d) — the clip after trimming cannot be larger than the original file",
				mode.ID, spec.MaxReferenceBytes, spec.MaxUploadBytes))
		}

		if caps.SupportsCloning && !declaresReferenceFormats(m, mode.ID) {
			warnings = append(warnings, fmt.Sprintf(
				"mode %q enables cloning but declares no reference_audio_formats; the platform will use default %v, which may not be what the Engine reads",
				mode.ID, PlatformDefaultAudioSpec.ReferenceAudioFormats))
		}
	}

	if m.Constraints.MaxTextLength < 0 {
		warnings = append(warnings, fmt.Sprintf(
			"constraints.max_text_length is negative (%d); the platform will use default %d",
			m.Constraints.MaxTextLength, DefaultMaxTextLength))
	}

	if r := m.Constraints.SpeedRange; r.Min > 0 && r.Max > 0 && r.Min > r.Max {
		warnings = append(warnings, fmt.Sprintf(
			"constraints.speed_range has min (%.2f) greater than max (%.2f)", r.Min, r.Max))
	}
	if r := m.Constraints.PitchRange; r.Min > r.Max {
		warnings = append(warnings, fmt.Sprintf(
			"constraints.pitch_range has min (%.2f) greater than max (%.2f)", r.Min, r.Max))
	}

	warnings = append(warnings, m.validateUISchema(seen)...)
	return errs, warnings
}

// declaresReferenceFormats reports whether the Engine ITSELF declared reference_audio_formats
// for this mode — at the mode level, or inherited from the Engine level.
//
// Does not ask ResolveAudioSpec: the resolver always falls back to the platform default, so it
// never returns an empty list, and a check based on that would never be true.
func declaresReferenceFormats(m *UniversalManifest, modeID string) bool {
	for i := range m.SupportedModes {
		if m.SupportedModes[i].ID == modeID {
			if len(m.SupportedModes[i].AudioSpec.ReferenceAudioFormats) > 0 {
				return true
			}
			break
		}
	}
	return len(m.AudioSpec.ReferenceAudioFormats) > 0
}

// validateUISchema catches ui_schema entries pointing to non-existent modes.
//
// Warning only, not an error: ui_schema is about presentation, so a typo here loses a control
// panel rather than producing incorrect audio. But silence would leave the author hunting
// forever for a control panel that never appears.
func (m *UniversalManifest) validateUISchema(modes map[string]struct{}) []string {
	if m.UISchema == nil {
		return nil
	}

	var out []string
	for _, id := range m.UISchema.ModelSort {
		if _, ok := modes[id]; !ok {
			out = append(out, fmt.Sprintf("ui_schema.model_sort references mode %q which is not in supported_modes", id))
		}
	}

	for id, panel := range m.UISchema.OptionPanel {
		if _, ok := modes[id]; !ok {
			out = append(out, fmt.Sprintf("ui_schema.option_panel has an entry for mode %q which is not in supported_modes", id))
			continue
		}
		// Preset voices declared on a mode that itself disables preset voices: the platform
		// will not show the list, so the author thinks they have finished configuring.
		if len(panel.PresetVoices) > 0 && !m.ResolveCapabilities(id).SupportsPresetVoices {
			out = append(out, fmt.Sprintf(
				"ui_schema.option_panel[%q] declares preset_voices but this mode resolves supports_preset_voices=false", id))
		}
	}
	return out
}
