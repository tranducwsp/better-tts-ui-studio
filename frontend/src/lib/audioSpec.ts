import type { AudioSpec, EngineModeSpec, UniversalManifest } from './types';

/**
 * Output-format helpers, described by the engine's manifest rather than assumed.
 * Shared so the settings panel and the streaming panel label audio identically.
 */

/** Applied when neither the mode nor the engine states a value. */
const PLATFORM_DEFAULTS: Required<
  Pick<
    AudioSpec,
    | 'supported_formats'
    | 'supported_sample_rates'
    | 'default_format'
    | 'default_sample_rate'
    | 'reference_audio_formats'
    | 'reference_audio_seconds'
    | 'max_upload_bytes'
    | 'max_reference_bytes'
  >
> = {
  supported_formats: ['wav'],
  supported_sample_rates: [24000],
  default_format: 'wav',
  default_sample_rate: 24000,
  reference_audio_formats: ['wav'],
  reference_audio_seconds: 5.0,
  max_upload_bytes: 100 * 1024 * 1024,
  max_reference_bytes: 10 * 1024 * 1024,
};

/**
 * Merge a mode's audio_spec over the engine-wide one.
 *
 * Modes can run on different backends — one streaming provider returning MP3, a local model
 * returning WAV — so a single engine-wide default_format would mislabel one of them. Same
 * precedence rule as capabilities: mode value wins, else engine value, else platform
 * default. Pass the mode wherever one is in play; omitting it resolves engine-wide.
 */
export function resolveAudioSpec(
  manifest: UniversalManifest | null | undefined,
  mode?: EngineModeSpec | string | null
): typeof PLATFORM_DEFAULTS {
  const engine = manifest?.audio_spec ?? {};

  let modeSpec: AudioSpec = {};
  if (mode && typeof mode === 'object') {
    modeSpec = mode.audio_spec ?? {};
  } else if (typeof mode === 'string' && mode) {
    modeSpec = (manifest?.supported_modes ?? []).find((m) => m.id === mode)?.audio_spec ?? {};
  }

  const list = <T>(...vals: (T[] | undefined)[]): T[] =>
    vals.find((v) => Array.isArray(v) && v.length > 0) as T[];
  const one = <T>(...vals: (T | undefined)[]): T =>
    vals.find((v) => v !== undefined && v !== null && v !== '' && v !== 0) as T;

  return {
    supported_formats: list(
      modeSpec.supported_formats,
      engine.supported_formats,
      PLATFORM_DEFAULTS.supported_formats
    ),
    supported_sample_rates: list(
      modeSpec.supported_sample_rates,
      engine.supported_sample_rates,
      PLATFORM_DEFAULTS.supported_sample_rates
    ),
    default_format: one(
      modeSpec.default_format,
      engine.default_format,
      PLATFORM_DEFAULTS.default_format
    ),
    default_sample_rate: one(
      modeSpec.default_sample_rate,
      engine.default_sample_rate,
      PLATFORM_DEFAULTS.default_sample_rate
    ),
    reference_audio_formats: list(
      modeSpec.reference_audio_formats,
      engine.reference_audio_formats,
      PLATFORM_DEFAULTS.reference_audio_formats
    ),
    reference_audio_seconds: one(
      modeSpec.reference_audio_seconds,
      engine.reference_audio_seconds,
      PLATFORM_DEFAULTS.reference_audio_seconds
    ),
    max_upload_bytes: one(
      modeSpec.max_upload_bytes,
      engine.max_upload_bytes,
      PLATFORM_DEFAULTS.max_upload_bytes
    ),
    max_reference_bytes: one(
      modeSpec.max_reference_bytes,
      engine.max_reference_bytes,
      PLATFORM_DEFAULTS.max_reference_bytes
    ),
  };
}

type Mode = EngineModeSpec | string | null | undefined;

/** e.g. "WAV 24kHz", "FLAC 48kHz", "MP3 22.1kHz". */
export function audioSpecLabel(manifest: UniversalManifest | null | undefined, mode?: Mode): string {
  const spec = resolveAudioSpec(manifest, mode);
  const fmt = spec.default_format.toUpperCase();
  const rate = spec.default_sample_rate;
  return rate ? `${fmt} ${(rate / 1000).toFixed(rate % 1000 === 0 ? 0 : 1)}kHz` : fmt;
}

/**
 * accept="" filter for reference-audio uploads.
 *
 * Reads reference_audio_formats, not supported_formats: the latter lists what the engine
 * *emits*, which is a different question from what it can *read*. An engine may return MP3
 * yet require WAV input, or the reverse.
 */
export function acceptedUploadFormats(manifest: UniversalManifest | null | undefined, mode?: Mode): string {
  return resolveAudioSpec(manifest, mode)
    .reference_audio_formats.map((f) => `.${f}`)
    .join(',');
}

/** Human-readable list of accepted upload formats, e.g. "WAV, MP3". */
export function supportedFormatsLabel(manifest: UniversalManifest | null | undefined, mode?: Mode): string {
  return resolveAudioSpec(manifest, mode)
    .reference_audio_formats.map((f) => f.toUpperCase())
    .join(', ');
}

/** The default output format — what a chunk's streamed blob actually contains. */
export function defaultFormat(manifest: UniversalManifest | null | undefined, mode?: Mode): string {
  return resolveAudioSpec(manifest, mode).default_format.toLowerCase();
}

/** MIME type for the default format, for assembling a combined download. */
export function defaultMimeType(manifest: UniversalManifest | null | undefined, mode?: Mode): string {
  const known: Record<string, string> = {
    wav: 'audio/wav',
    mp3: 'audio/mpeg',
    flac: 'audio/flac',
    ogg: 'audio/ogg',
    opus: 'audio/opus',
    aac: 'audio/aac',
    m4a: 'audio/mp4',
  };
  const fmt = defaultFormat(manifest, mode);
  return known[fmt] ?? `audio/${fmt}`;
}

/**
 * Formats a finished chunk can be downloaded as, in display order.
 *
 * The default comes first and is served from the blob already downloaded; the rest are
 * fetched from the backend, which transcodes on request.
 */
export function downloadFormats(manifest: UniversalManifest | null | undefined, mode?: Mode): string[] {
  const spec = resolveAudioSpec(manifest, mode);
  const def = spec.default_format.toLowerCase();
  const declared = spec.supported_formats.map((f) => f.toLowerCase());
  return [def, ...declared.filter((f) => f !== def)];
}

/**
 * Format used for the "download everything as one file" action.
 *
 * Concatenating chunks only yields a playable file for formats that tolerate appending
 * frames, which rules out container formats like WAV — its header declares a length, so a
 * concatenated WAV is truncated to the first chunk in most players.
 *
 * Returns null when the engine offers no concatenation-safe format, in which case the
 * caller should not offer a combined download at all.
 */
export function combinedDownloadFormat(
  manifest: UniversalManifest | null | undefined,
  mode?: Mode
): string | null {
  const declared = downloadFormats(manifest, mode);
  return ['mp3', 'aac'].find((f) => declared.includes(f)) ?? null;
}

const asMB = (bytes: number): string => {
  const mb = bytes / (1024 * 1024);
  return `${Number.isInteger(mb) ? mb : mb.toFixed(1)} MB`;
};

/**
 * Ceiling for the raw file a user picks, before trimming.
 *
 * Higher than maxReferenceBytes on purpose: someone may drop in a half-hour recording and
 * keep four seconds of it, so rejecting at the file picker on the engine's own limit would
 * refuse a file that is perfectly fine to trim.
 */
export function maxUploadBytes(manifest: UniversalManifest | null | undefined, mode?: Mode): number {
  return resolveAudioSpec(manifest, mode).max_upload_bytes;
}

/** maxUploadBytes rendered for a label, e.g. "100 MB". */
export function maxUploadLabel(manifest: UniversalManifest | null | undefined, mode?: Mode): string {
  return asMB(maxUploadBytes(manifest, mode));
}

/** Ceiling for the trimmed clip the engine actually receives. */
export function maxReferenceBytes(manifest: UniversalManifest | null | undefined, mode?: Mode): number {
  return resolveAudioSpec(manifest, mode).max_reference_bytes;
}

/** maxReferenceBytes rendered for a label, e.g. "10 MB". */
export function maxReferenceLabel(manifest: UniversalManifest | null | undefined, mode?: Mode): string {
  return asMB(maxReferenceBytes(manifest, mode));
}

/** Seconds of reference audio the engine wants — the window the trimmer selects. */
export function referenceAudioSeconds(manifest: UniversalManifest | null | undefined, mode?: Mode): number {
  return resolveAudioSpec(manifest, mode).reference_audio_seconds;
}
