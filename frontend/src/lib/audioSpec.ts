import type { UniversalManifest } from './types';

/**
 * Output-format helpers, described by the engine's manifest rather than assumed.
 * Shared so the settings panel and the streaming panel label audio identically.
 */

/** e.g. "WAV 24kHz", "FLAC 48kHz", "MP3 22.1kHz". */
export function audioSpecLabel(manifest: UniversalManifest | null | undefined): string {
  const spec = manifest?.audio_spec;
  const fmt = (spec?.default_format ?? 'wav').toUpperCase();
  const rate = spec?.default_sample_rate;
  return rate ? `${fmt} ${(rate / 1000).toFixed(rate % 1000 === 0 ? 0 : 1)}kHz` : fmt;
}

/** accept="" filter for reference-audio uploads, e.g. ".wav,.mp3". */
export function acceptedUploadFormats(manifest: UniversalManifest | null | undefined): string {
  const formats = manifest?.audio_spec?.supported_formats;
  if (!formats || formats.length === 0) return '.wav,audio/wav';
  return formats.map((f) => `.${f}`).join(',');
}

/** Human-readable list, e.g. "WAV, MP3". */
export function supportedFormatsLabel(manifest: UniversalManifest | null | undefined): string {
  return (manifest?.audio_spec?.supported_formats ?? ['wav']).map((f) => f.toUpperCase()).join(', ');
}

/**
 * The default output format, lowercased — what a chunk's streamed blob actually contains.
 */
export function defaultFormat(manifest: UniversalManifest | null | undefined): string {
  return (manifest?.audio_spec?.default_format ?? 'wav').toLowerCase();
}

/** MIME type for the default format, for assembling a combined download. */
export function defaultMimeType(manifest: UniversalManifest | null | undefined): string {
  const fmt = defaultFormat(manifest);
  const known: Record<string, string> = {
    wav: 'audio/wav',
    mp3: 'audio/mpeg',
    flac: 'audio/flac',
    ogg: 'audio/ogg',
    opus: 'audio/opus',
    aac: 'audio/aac',
    m4a: 'audio/mp4',
  };
  return known[fmt] ?? `audio/${fmt}`;
}

/**
 * Formats a finished chunk can be downloaded as, in display order.
 *
 * The default format comes first and is served from the already-downloaded blob; the rest
 * are fetched from the backend, which transcodes on request. Engines that declare no
 * audio_spec fall back to their default format alone rather than assuming WAV exists.
 */
export function downloadFormats(manifest: UniversalManifest | null | undefined): string[] {
  const def = defaultFormat(manifest);
  const declared = (manifest?.audio_spec?.supported_formats ?? []).map((f) => f.toLowerCase());
  if (declared.length === 0) return [def];
  return [def, ...declared.filter((f) => f !== def)];
}

/**
 * Format used for the "download everything as one file" action.
 *
 * Concatenating chunks only yields a playable file for formats that tolerate simply
 * appending frames, which rules out container formats like WAV — its header declares a
 * length, so a concatenated WAV is truncated to the first chunk in most players. MP3 has
 * no such header, and a long recording is what people actually want compressed anyway.
 *
 * Returns null when the engine cannot produce a concatenation-safe format, in which case
 * the caller should not offer a combined download at all.
 */
export function combinedDownloadFormat(
  manifest: UniversalManifest | null | undefined
): string | null {
  const declared = downloadFormats(manifest);
  const safe = ['mp3', 'aac'];
  return safe.find((f) => declared.includes(f)) ?? null;
}
