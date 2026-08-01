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

/**
 * Delimiters the engine wants text split on, as [paragraphBoundary, sentenceBoundary].
 *
 * Both are used with String.split(), so zero-width lookbehind patterns like
 * `(?<=[.!?]\s+)` are the expected shape — they mark a cut point without consuming
 * characters. A content-matching pattern still works, but as a split it drops the matched
 * delimiter itself, so lookbehind is preferred.
 */
function resolveDelimiters(manifest: UniversalManifest | null | undefined): [string, string] {
  const d = manifest?.ui_schema?.input_panel?.chunk_delimiters;
  return [d?.[0] || '\\n\\n', d?.[1] || '(?<=[.!?]\\s+)'];
}

function compile(pattern: string, fallback: RegExp): RegExp {
  try {
    return new RegExp(pattern);
  } catch {
    return fallback;
  }
}

/**
 * THE chunk splitter. Both the preview under the textarea and the chunks actually sent to
 * the backend call this, so the user can never be shown one division while a different
 * one is transmitted.
 *
 * Guarantees, in order of importance:
 *   1. No chunk exceeds resolveChunkSize(manifest); anything longer is rejected by the
 *      backend's own manifest validation.
 *   2. No character of the input is lost. Fragments that survive neither delimiter are
 *      hard-split rather than discarded — which is why split() is used throughout instead
 *      of match(): a match-based pass silently drops whatever fails to match, and a
 *      trailing sentence with no terminating punctuation is exactly that case.
 *   3. Cuts land on engine-declared boundaries where possible, falling back to paragraph,
 *      then sentence, then a hard cut.
 */
export function splitIntoChunks(
  text: string,
  manifest: UniversalManifest | null | undefined
): string[] {
  if (!text) return [];
  const limit = resolveChunkSize(manifest);
  if (text.length <= limit) return [text];

  const [paraPattern, sentPattern] = resolveDelimiters(manifest);
  const paraRegex = compile(paraPattern, /\n\n/);
  const sentRegex = compile(sentPattern, /(?<=[.!?]\s+)/);

  const chunks: string[] = [];
  let current = '';

  const flush = () => {
    if (current) {
      chunks.push(current);
      current = '';
    }
  };

  const append = (piece: string, joiner: string) => {
    if (!current) {
      current = piece;
    } else if (current.length + joiner.length + piece.length <= limit) {
      current += joiner + piece;
    } else {
      flush();
      current = piece;
    }
  };

  for (const rawPara of text.split(paraRegex)) {
    const para = rawPara.trim();
    if (!para) continue;

    if (para.length <= limit) {
      append(para, '\n');
      continue;
    }

    // Too big for one chunk: fall to sentence boundaries.
    for (const rawSent of para.split(sentRegex)) {
      const sent = rawSent.trim();
      if (!sent) continue;

      if (sent.length <= limit) {
        append(sent, ' ');
        continue;
      }

      // A single sentence longer than the limit, with no usable delimiter inside.
      // Hard-split so nothing is lost and nothing exceeds the ceiling.
      flush();
      for (let i = 0; i < sent.length; i += limit) {
        chunks.push(sent.slice(i, i + limit));
      }
    }
  }

  flush();
  return chunks.length ? chunks : [text.slice(0, limit)];
}

