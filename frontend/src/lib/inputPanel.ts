import type { InputPanelSpec, ResolvedInputPanel } from './types';

/**
 * Single source of truth for "does the input panel have feature X?".
 *
 * The engine may omit bool fields in input_panel; `undefined` means "inherit the
 * platform default" while `false` means "explicitly disabled". This module resolves
 * the two-tier priority: engine value → platform default.
 *
 * Everything that needs an input-panel feature must call resolveInputPanel. Reading
 * inputPanelSpec directly reintroduces the drift this module exists to prevent.
 */

/** Applied when the engine does not state a value. Must match PlatformDefaultInputPanel in Go. */
export const PLATFORM_DEFAULT_INPUT_PANEL: ResolvedInputPanel = {
  file_serve: true,
  find_mode: 'expert',
  replace_tool: true,
  enable_chunk_box: true,
};

function pickBool(
  engine: boolean | undefined,
  fallback: boolean
): boolean {
  if (typeof engine === 'boolean') return engine;
  return fallback;
}

function pickStr(
  engine: string | undefined,
  fallback: string
): string {
  if (engine) return engine;
  return fallback;
}

/**
 * Merge the engine's input_panel spec with platform defaults.
 *
 * Returns the full resolved spec, never null. When inputPanelSpec is null/undefined,
 * all platform defaults apply.
 */
export function resolveInputPanel(
  inputPanelSpec?: InputPanelSpec | null
): ResolvedInputPanel {
  if (!inputPanelSpec) {
    return { ...PLATFORM_DEFAULT_INPUT_PANEL };
  }

  const d = PLATFORM_DEFAULT_INPUT_PANEL;
  return {
    file_serve: pickBool(inputPanelSpec.file_serve, d.file_serve),
    find_mode: pickStr(inputPanelSpec.find_mode, d.find_mode),
    replace_tool: pickBool(inputPanelSpec.replace_tool, d.replace_tool),
    enable_chunk_box: pickBool(inputPanelSpec.enable_chunk_box, d.enable_chunk_box),
    auto_format: inputPanelSpec.auto_format,
  };
}
