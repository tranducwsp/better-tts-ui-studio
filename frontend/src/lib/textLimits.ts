import type { UniversalManifest } from './types';

/**
 * Single source of truth for every text-length decision in the app.
 *
 * The manifest describes two related but distinct numbers:
 *   - `constraints.max_text_length` — the hard ceiling the backend enforces on ONE
 *     synthesize request. Exceeding it is a 400 from the engine.
 *   - `ui_schema.input_panel.max_chunk_size` — an optional, purely presentational
 *     preference for how finely the engine would like text sliced.
 *
 * Chunk size must therefore never exceed max_text_length, even if an engine declares a
 * larger max_chunk_size. Resolving this in one place keeps the chunk preview the user
 * sees, the chunks actually sent, and the streaming threshold from drifting apart.
 */

/** Used only when no manifest has loaded yet. */
export const DEFAULT_TEXT_LIMIT = 3000;

/** Hard per-request ceiling enforced by the backend. */
export function resolveMaxTextLength(manifest: UniversalManifest | null | undefined): number {
  const v = manifest?.constraints?.max_text_length;
  return typeof v === 'number' && v > 0 ? v : DEFAULT_TEXT_LIMIT;
}

/**
 * Size of a single chunk. Clamped to the per-request ceiling so a generous
 * `max_chunk_size` can never produce a chunk the backend will reject.
 */
export function resolveChunkSize(manifest: UniversalManifest | null | undefined): number {
  const ceiling = resolveMaxTextLength(manifest);
  const preferred = manifest?.ui_schema?.input_panel?.max_chunk_size;
  if (typeof preferred === 'number' && preferred > 0) {
    return Math.min(preferred, ceiling);
  }
  return ceiling;
}

/** Text longer than this is routed through chunked streaming instead of one request. */
export function resolveStreamingThreshold(manifest: UniversalManifest | null | undefined): number {
  return resolveMaxTextLength(manifest);
}
