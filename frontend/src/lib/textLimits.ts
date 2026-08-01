import type { UniversalManifest } from './types';

/**
 * Single source of truth for every text-length decision in the app.
 *
 * The manifest describes two related but distinct numbers:
 *   - `constraints.max_text_length` — the hard ceiling the backend enforces on ONE
 *     synthesize request. Exceeding it is a 400 from the engine.
 *   - `constraints.chunking.max_chunk_size` — an optional preference for how finely the
 *     engine would like text sliced, plus the delimiters to slice on.
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
  const preferred = manifest?.constraints?.chunking?.max_chunk_size;
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
function resolveDelimiters(manifest: UniversalManifest | null | undefined): string[] {
  const d = manifest?.constraints?.chunking?.delimiters;
  return d && d.length > 0 ? d : ['\\n\\n', '(?<=[.!?]\\s+)'];
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
 * the backend call this, so the user can never be shown one division while a different one
 * is transmitted.
 *
 * Guarantees, in order of importance:
 *   1. No chunk exceeds resolveChunkSize(manifest); anything longer is rejected by the
 *      backend's own manifest validation.
 *   2. No character of the input is lost. Fragments that survive every delimiter are
 *      hard-split rather than discarded — which is why split() is used throughout instead
 *      of match(): a match-based pass silently drops whatever fails to match, and a
 *      trailing sentence with no terminating punctuation is exactly that case.
 *   3. Cuts land on engine-declared boundaries where possible, trying each delimiter in
 *      declared order before resorting to a hard cut.
 *
 * The delimiter list is walked recursively rather than as a fixed paragraph/sentence pair,
 * so an engine may declare one boundary or five and each is honoured.
 */
export function splitIntoChunks(
  text: string,
  manifest: UniversalManifest | null | undefined
): string[] {
  if (!text) return [];
  const limit = resolveChunkSize(manifest);
  if (text.length <= limit) return [text];

  const patterns = resolveDelimiters(manifest);
  const regexes = patterns.map((p, i) =>
    compile(p, i === 0 ? /\n\n/ : /(?<=[.!?]\s+)/)
  );

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

  /**
   * Emit one fragment, cutting it further with delimiter[depth] if it is still too long.
   * Falls through to a hard cut once every declared delimiter has been tried.
   */
  const emit = (fragment: string, depth: number, joiner: string) => {
    const piece = fragment.trim();
    if (!piece) return;

    if (piece.length <= limit) {
      append(piece, joiner);
      return;
    }

    if (depth >= regexes.length) {
      // No delimiter left to try: cut on length so nothing is lost or oversized.
      flush();
      for (let i = 0; i < piece.length; i += limit) {
        chunks.push(piece.slice(i, i + limit));
      }
      return;
    }

    const parts = piece.split(regexes[depth]);
    // A delimiter that does not appear in this fragment yields a single part; recursing on
    // it with the same depth would loop forever, so move to the next delimiter.
    if (parts.length <= 1) {
      emit(piece, depth + 1, joiner);
      return;
    }

    // Deeper levels join with a space; only the outermost boundary implies a line break.
    const childJoiner = depth === 0 ? '\n' : ' ';
    for (const part of parts) {
      emit(part, depth + 1, childJoiner);
    }
  };

  for (const part of text.split(regexes[0])) {
    emit(part, 1, '\n');
  }

  flush();
  return chunks.length ? chunks : [text.slice(0, limit)];
}
