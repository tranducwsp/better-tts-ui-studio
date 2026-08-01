# Platform Gaps & Known Limitations

Running list of places where the platform does not yet fully honour the manifest, or
where a manifest field exists but nothing consumes it. Kept so an AI engineer plugging in
a new Core TTS engine knows what the platform will and will not do for them today.

Last updated: 2026-08-01

---

## 1. `capabilities.supports_ssml` — declared but unused

**Status:** type exists (`types/manifest.go`, `frontend/src/lib/types.ts`), nothing reads it.

An engine can set `supports_ssml: true` and nothing happens. Text is always sent as plain
text.

Not implemented because it needs real design decisions rather than a mechanical wiring:

- Does the user get a separate SSML editor, a toggle on the existing textarea, or
  automatic passthrough?
- Who validates the markup — frontend, backend, or the engine? Malformed SSML should
  fail with a useful message, not a generic 400.
- When the active mode does *not* support SSML, should the backend strip tags, reject the
  request, or forward as-is?
- Chunk splitting currently slices on sentence boundaries. Splitting SSML that way can cut
  an open tag from its close tag — chunking would need to become markup-aware.

Wire it only after those are settled.

## 2. Reference-audio upload size limit is hardcoded

`GenericEnginePanel.svelte` tells the user "Max 10MB". The manifest has no field
describing an upload ceiling, so this number is a frontend invention and is not enforced
anywhere in the backend either.

To make it engine-driven, add something like `audio_spec.max_upload_bytes` to
`types/manifest.go` and the core protocol spec, then read it in the dropzone label and
enforce it in `handlers/tts_clone.go`.

## 3. Pitch / emotion depend on the engine actually reading them

The platform side is complete: UI gated by manifest, validated against
`pitch_range` / `supported_emotions`, persisted to `tts_jobs`, restored from history, and
forwarded to the engine as `pitch` / `emotion` in the `/synthesize` payload.

But the Core TTS engine has to *use* those fields. If it ignores them the user will move
the slider and hear no difference — the gap is then in the engine, not here.

Fields are omitted entirely (not sent as null) when the engine does not declare support,
so engines that predate this feature are unaffected.

## 4. History does not restore reference audio for cloning jobs

`JobDetailResponse` returns voice, speed, pitch, emotion and chunks. For a cloning job the
`voice` field holds a temp reference-audio path that has usually been cleaned off disk by
the time the job is reloaded, so replaying such a job may fail.

Fixing this means deciding a retention policy for reference audio — out of scope so far.

## 5. `supported_sample_rates` is informational only

`audio_spec.default_format` and `default_sample_rate` drive the result-card label, and
`supported_formats` drives the upload filter. The `supported_sample_rates` array is not
used — there is no UI for choosing an output sample rate, and no request field to carry
that choice. Add one to the synthesize payload first if this should be selectable.

---

## Resolved: capability and chunking fragmentation

Recorded here because the shape of the fix matters for anything added later.

**Capabilities used to be stated in three places.** `capabilities` carried engine-wide
booleans, `supported_modes[]` carried four *different* flat booleans, and `option_panel`
carried `pitch_type`/`emotion_type`. The three sets neither matched nor complemented each
other: `supports_voice_saving` existed only per mode, `supports_pitch` only engine-wide, and
three flags existed in both with no stated precedence. The frontend had to invent a
reconciliation rule per call site — including a `modeGate()` helper that inferred whether a
mode supported pitch from whether it declared a *widget* for it, which is presentation data
answering a capability question.

Now `EngineCapabilities` has one shape used at both levels, every field nullable, and one
rule: mode value wins, else engine value, else platform default. Nullability is what makes
it work — `false` means "this mode cannot", `undefined` means "inherit". Resolution lives in
exactly two places, `types.ResolveCapabilities` (Go) and `lib/capabilities.ts` (frontend),
which are kept behaviourally identical and tested against the same cases.

**Chunking moved from `ui_schema.input_panel` to `constraints.chunking`.** It decides what
is transmitted, not how anything looks, and it needs clamping against `max_text_length`
which lives under `constraints`. `chunk_delimiters` also became `delimiters` and is now
walked recursively, so an engine may declare one boundary or five instead of the previous
hardcoded pair where a third entry was silently ignored.

**`preset_voices` gained a type.** It was `[]map[string]string`, which forced the frontend
to cast to `Record<string, unknown>` to read `gender`. Now `PresetVoiceSpec`.

## Text length and chunking: single source of truth

Chunk sizing used to be decided in three places with three different fallbacks, and the
splitting *algorithm* lived in two places with different regexes — so the preview under
the textarea could show a different division than the one actually transmitted.

All of it now resolves through `frontend/src/lib/textLimits.ts`:

| Concern | Function | Source |
|---|---|---|
| Per-request ceiling | `resolveMaxTextLength()` | `constraints.max_text_length` |
| Chunk size | `resolveChunkSize()` | `constraints.chunking.max_chunk_size`, **clamped** to the ceiling |
| Streaming threshold | `resolveStreamingThreshold()` | `constraints.max_text_length` |
| **How text is cut** | `splitIntoChunks()` | `constraints.chunking.delimiters` |

`TextInputPanel` (the preview box) and `StreamingPanel` (what is sent) both call
`splitIntoChunks(text, manifest)`, so they cannot disagree — it is the same call.

Two bugs this fixed, both confirmed by measurement:

- The engine declares `chunk_delimiters` as zero-width lookbehinds, e.g.
  `(?<=[.!?]\s+)`. The preview fed the second one to `.match()`, which returns an array of
  empty strings for a zero-width pattern; every one was then skipped by the `if (!clean)
  continue` guard. Any paragraph longer than `limit * 2` therefore fell through
  unsplit — the preview claimed "1 chunk" for text that was 4209 characters against a
  300-character limit.
- `StreamingPanel` ignored `chunk_delimiters` entirely and hardcoded
  `[^.!?]+[.!?]+` with `.match()`. Because `.match()` discards whatever fails to match, a
  trailing sentence without terminating punctuation was dropped: 22 characters lost from a
  3152-character input.

`splitIntoChunks` uses `split()` for both tiers, which is the shape lookbehind delimiters
are written for, and hard-splits any fragment still over the limit. Verified across 35
combinations of manifest shape and text shape: preview is byte-identical to what is sent,
no characters are lost, and no chunk exceeds the ceiling — including when the engine
declares a syntactically invalid delimiter, when `max_chunk_size` exceeds
`max_text_length`, and when no manifest has loaded.

The clamp matters independently: an engine declaring `max_chunk_size: 9999` alongside
`max_text_length: 500` still gets 500-char chunks, because anything larger is rejected by
the backend's own manifest validation.

Fallback is 3000 and applies only when no manifest has loaded.

When adding a new text-length or chunking decision, call into `textLimits.ts` — do not
read the manifest directly, or these will drift apart again.
