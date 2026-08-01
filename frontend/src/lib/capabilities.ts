import type {
  EngineCapabilities,
  EngineModeSpec,
  ResolvedCapabilities,
  UniversalManifest,
} from './types';

/**
 * Single source of truth for "can this mode do X?".
 *
 * The manifest states capabilities on two levels: engine-wide defaults, and per-mode
 * overrides. A mode's own value wins when it states one; otherwise the engine-wide value
 * applies; otherwise the platform default below. `undefined` therefore means "inherit"
 * while `false` means "explicitly cannot" — collapsing the two would make it impossible
 * for a mode to opt out of something the engine enables.
 *
 * Everything that needs a capability must call this. Reading manifest.capabilities
 * directly reintroduces the drift this module exists to prevent.
 */

/** Applied when neither the mode nor the engine states a value. */
const PLATFORM_DEFAULTS: ResolvedCapabilities = {
  supports_preset_voices: true,
  supports_cloning: false,
  supports_voice_saving: false,
  supports_streaming: true,
  supports_speed: true,
  supports_pitch: false,
  supports_emotion: false,
  supports_ssml: false,
};

function pick(
  mode: boolean | undefined,
  engine: boolean | undefined,
  fallback: boolean
): boolean {
  if (typeof mode === 'boolean') return mode;
  if (typeof engine === 'boolean') return engine;
  return fallback;
}

/**
 * Merge a mode's capabilities over the engine-wide set.
 *
 * `mode` may be the mode spec itself or just its id; an unknown id or null resolves to the
 * engine-wide values, which is what a not-yet-loaded manifest should look like.
 */
export function resolveCapabilities(
  manifest: UniversalManifest | null | undefined,
  mode?: EngineModeSpec | string | null
): ResolvedCapabilities {
  const engine: EngineCapabilities = manifest?.capabilities ?? {};

  let modeCaps: EngineCapabilities = {};
  if (mode && typeof mode === 'object') {
    modeCaps = mode.capabilities ?? {};
  } else if (typeof mode === 'string' && mode) {
    const found = (manifest?.supported_modes ?? []).find((m) => m.id === mode);
    modeCaps = found?.capabilities ?? {};
  }

  return {
    supports_preset_voices: pick(
      modeCaps.supports_preset_voices,
      engine.supports_preset_voices,
      PLATFORM_DEFAULTS.supports_preset_voices
    ),
    supports_cloning: pick(
      modeCaps.supports_cloning,
      engine.supports_cloning,
      PLATFORM_DEFAULTS.supports_cloning
    ),
    supports_voice_saving: pick(
      modeCaps.supports_voice_saving,
      engine.supports_voice_saving,
      PLATFORM_DEFAULTS.supports_voice_saving
    ),
    supports_streaming: pick(
      modeCaps.supports_streaming,
      engine.supports_streaming,
      PLATFORM_DEFAULTS.supports_streaming
    ),
    supports_speed: pick(
      modeCaps.supports_speed,
      engine.supports_speed,
      PLATFORM_DEFAULTS.supports_speed
    ),
    supports_pitch: pick(
      modeCaps.supports_pitch,
      engine.supports_pitch,
      PLATFORM_DEFAULTS.supports_pitch
    ),
    supports_emotion: pick(
      modeCaps.supports_emotion,
      engine.supports_emotion,
      PLATFORM_DEFAULTS.supports_emotion
    ),
    supports_ssml: pick(
      modeCaps.supports_ssml,
      engine.supports_ssml,
      PLATFORM_DEFAULTS.supports_ssml
    ),
  };
}
