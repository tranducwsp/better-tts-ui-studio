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

## Text length: single source of truth

Chunk sizing used to be decided in three places with three different fallbacks, which let
the chunk preview disagree with the chunks actually sent. All of it now resolves through
`frontend/src/lib/textLimits.ts`:

| Concern | Resolver | Source |
|---|---|---|
| Per-request ceiling | `resolveMaxTextLength()` | `constraints.max_text_length` |
| Chunk size | `resolveChunkSize()` | `input_panel.max_chunk_size`, **clamped** to the ceiling |
| Streaming threshold | `resolveStreamingThreshold()` | `constraints.max_text_length` |

The clamp matters: an engine declaring `max_chunk_size: 9999` alongside
`max_text_length: 500` still gets 500-char chunks, because anything larger is rejected by
the backend's own manifest validation.

Fallback is 3000 and applies only when no manifest has loaded.

When adding a new text-length decision, call a resolver — do not read the manifest
directly, or the three numbers will drift apart again.
