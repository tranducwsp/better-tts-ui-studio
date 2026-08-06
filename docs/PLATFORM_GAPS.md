# Platform Gaps & Known Limitations

Running list of places where the platform does not yet fully honour the manifest, or
where a manifest field exists but nothing consumes it. Kept so an AI engineer plugging in
a new Core TTS engine knows what the platform will and will not do for them today.

Last updated: 2026-08-07

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

## 2. Pitch / emotion depend on the engine actually reading them

The platform side is complete: UI gated by manifest, validated against
`pitch_range` / `supported_emotions`, persisted to `tts_jobs`, restored from history, and
forwarded to the engine as `pitch` / `emotion` in the `/synthesize` payload.

But the Core TTS engine has to *use* those fields. If it ignores them the user will move
the slider and hear no difference — the gap is then in the engine, not here.

Fields are omitted entirely (not sent as null) when the engine does not declare support,
so engines that predate this feature are unaffected.

## 3. History does not restore reference audio for cloning jobs

`JobDetailResponse` returns voice, speed, pitch, emotion and chunks. For a cloning job the
`voice` field holds a temp reference-audio path that has usually been cleaned off disk by
the time the job is reloaded, so replaying such a job may fail.

Fixing this means deciding a retention policy for reference audio — out of scope so far.

## 4. `supported_sample_rates` is informational only

`audio_spec.default_format` and `default_sample_rate` drive the result-card label, and
`supported_formats` drives the upload filter. The `supported_sample_rates` array is not
used — there is no UI for choosing an output sample rate, and no request field to carry
that choice. Add one to the synthesize payload first if this should be selectable.

## 5. Per-process ceilings multiply across replicas

Hai trần dưới đây đúng cho một tiến trình và sai ngay khi có replica thứ hai. Cả hai đều
KHÔNG hỏng dữ liệu — chúng chỉ tiêu tốn nhiều hơn mức người đọc mã nguồn tưởng.

**`transcodeSlots` (`audio/transcode.go`)** — `max(2, GOMAXPROCS(0))` chỗ cho ffmpeg, đếm
theo tiến trình. Comment ở đó nói nó chặn "một người dùng ghim mọi nhân", và điều đó chỉ
đúng với một replica web. Bốn replica trên máy tám nhân là 32 tiến trình ffmpeg giành tám
nhân, mỗi tiến trình giữ cả đầu vào và đầu ra trong RAM. Thêm nữa `GOMAXPROCS` báo số nhân
của MÁY, không phải phần cgroup được cấp, nên trần còn nở ra theo kích thước host.

**Sweeper (`storage/sweeper.go`)** — chạy trong worker, một goroutine mỗi tiến trình. Đã
lấy ra khỏi web đúng vì lý do này, nhưng lấy sang worker thì vấn đề chỉ dịch chỗ: bốn
worker replica là bốn lượt `LIST` toàn nhánh `temp/` mỗi giờ để xoá đúng cùng một tập tệp.
Không hỏng vì `Delete` một khoá không tồn tại không phải lỗi (hợp đồng của `Store`), nhưng
với S3 thì đó là request và tiền.

Sweeper là **cron**, không phải worker: nó kích hoạt bởi đồng hồ chứ không bởi hàng đợi,
nên chạy nhiều bản không chia được việc cho nhau như worker chia job. Nó đang nằm trong
worker vì đó là tiến trình duy nhất không phải web.

Chưa xử lý vì hiện chỉ chạy một replica mỗi loại, và cả hai chỉ tốn thêm tài nguyên chứ
không sai kết quả. Khi thật sự scale:

- `transcodeSlots` — chuyển sang trần dùng chung trên Redis (cùng khuôn với rate limiter đã
  chuyển), hoặc đọc quota cgroup thay cho `GOMAXPROCS`.
- Sweeper — cho nó giành khoá Redis (`SET lock:sweeper <id> NX EX`) rồi mới quét, để N
  replica thì đúng một đứa chạy mỗi giờ. Cách này đúng với mọi số replica, kể cả khi ai đó
  scale lên — khác với việc tách thành service riêng, nơi tính đúng đắn phụ thuộc vào việc
  người vận hành nhớ giữ đúng một bản.

---

## Resolved: one audio format declared for modes that emit different ones

`audio_spec` was engine-wide only, so `default_format: "wav"` applied to every mode. The
bundled engine drives `fast` through Edge TTS, which returns MP3 — the file was served as
`Content-Type: audio/wav` with a `.wav` name while containing MP3 frames, confirmed by the
`0xFF 0xF3` header on a real download. Many players refuse that.

`audio_spec` now resolves per mode, same two-tier rule as capabilities, through
`types.ResolveAudioSpec` and `resolveAudioSpec` in `lib/audioSpec.ts`.

`TaskItem.AudioWAV` was renamed to `Audio` with a `SourceFormat` beside it: the old name
asserted a format the task could not guarantee, and every reader inherited that assumption.
Transcoded copies moved from a single `AudioMP3` field to a map keyed by format, so caching
works for whatever the engine actually produces.

Measured after: `fast` returns `audio/mpeg` with MP3 magic bytes, `?format=wav` transcodes
to real RIFF, and `standard` is unaffected.

## Resolved: /voices ignored which mode was asking

`GET /voices` took no parameters, so every mode received the same list. On the bundled
engine that meant 16 voices everywhere, when `fast` is driven by Edge TTS and can only use
2 of them — the other 14 were selectable and would have failed at synthesis time. The engine
already knew the split; it just had no way to express it.

`GET /voices?model_id=` filters now, and `VoiceInfo.modes` is required — no "applies
everywhere" default, because that is how a voice ends up offered where it does not work.

Filtering is the engine's job alone. The platform passes the parameter and trusts the
answer; it does not re-filter. An earlier draft did both, to cover an engine that declares
`modes` but ignores the query — that is defensive code for a contract violation, and it
would have outlived the memory of why it exists.

Measured after the change: `fast` returns 2, `standard` 14, `clone` 17 (14 presets plus 3
saved user voices).

## Resolved: settings that were read but never enforced

Three values looked configurable and were not.

**`MAX_UPLOAD_SIZE_MB` was parsed, range-checked, logged at boot — and ignored.** All three
multipart handlers hardcoded `32 << 20`, so setting it to 8 or 512 changed nothing. Uploads
now go through `parseUpload`, which also wraps the body in `http.MaxBytesReader` so an
oversized file is cut off mid-transfer instead of being buffered into memory and then
refused. The error names the limit, because "request too large" without a number leaves the
user guessing how much to trim.

The engine can tighten it further through the new `audio_spec.max_upload_bytes`: whichever
of the two is stricter wins. The engine knows what it can process; the env var is the
infrastructure ceiling.

**`STORAGE_DIR` was honoured at startup and nowhere else.** `main.go` created the directory
and pointed the sweeper at it, but the handlers wrote to a literal `"storage/temp"`. Setting
`STORAGE_DIR=/data` therefore had the sweeper cleaning `/data/temp` while synthesis kept
filling `./storage/temp`, which nothing would ever delete. Paths now come from
`storage.TempDir()` and `storage.ModeDir()`, set once from config at boot.

**The waveform trimmer always cut exactly 5 seconds.** Reference-clip length is an engine
property — some want three seconds, some ten — so it is now
`audio_spec.reference_audio_seconds`, and the surrounding labels read from the same value
instead of saying "5s" in four places.

Verified against the running deployment: a 15 MB upload is refused with "tệp vượt quá giới
hạn 10 MB" from the engine's declared ceiling, while 1 MB passes through to the engine.

## Resolved: mode names were guessed in six places

The platform assumed an engine would name its modes `standard`, `fast` and `clone`.

The one that actually broke: `cloneVoiceTemp()` and `POST /api/clone/upload` both fell back
to the literal `"clone"` when no `model_id` arrived. An engine calling its cloning mode
`zero_shot_clone` — as the bundled test engine does — had reference audio filed under a
`model_id` no mode owned, so `/api/clone/voices?model_id=zero_shot_clone` returned nothing
and the voice was unreachable from the UI that created it. The frontend never passed
`model_id` on the temp-upload path at all, so the fallback ran every time.

`handlers/tts_clone.go` now asks the manifest which mode declares `supports_cloning`, and
the frontend passes `activeMode.id`. Verified against an engine with no mode named `clone`:
a voice uploaded without `model_id` lands under `zero_shot_clone`.

Also removed, all of the same shape:

- `activeTab` started at `"fast"`, rendering a tab for a mode that may not exist until an
  effect corrected it. It starts empty; the panel waits for the engine to report modes.
- `fetchVoices()`, `synthesize()` and `cloneVoice()` defaulted their mode parameter. Every
  caller already passes one, so the defaults only hid a missing argument — they are now
  required.
- `CreateVoiceModal` seeded its form with `Male` / `Northern` / `Expressive` and offered
  Vietnamese regional accents as fallback options. An engine that declares no
  `voice_metadata_schema` now gets a name field only, and each field seeds from its own
  schema rather than from a guess about language.

What remains is deliberate: `unified.go` falls back to `"standard"` only when the manifest
has not loaded and there is no mode list to read.

## Resolved: shared secret default and unbounded temp storage

Two things that made the stack unsafe to expose, both fixed.

**`SECRET_KEY` had a hardcoded default.** `config.go` fell back to
`"default_secret_key_change_me"`, and that key signs JWT session tokens. Anyone who read the
repository could forge a valid admin token without a password or database access. The
seed-account passwords were literals in the same file.

There is no default now, and no fallback either. An unset `SECRET_KEY` used to generate a
random key and log a warning — safer than a shared literal, but it still let the process boot
into a state nobody chose: sessions dropped on every restart and replicas could not share
them, announced only by a log line that scrolls away. `requireAll()` refuses to start instead,
listing every missing required variable at once. Seed accounts default to empty, and
`seedDefaultAccounts` already skipped entries missing a username or password, so an
unconfigured deployment simply creates no accounts and the first user registers through the UI.

`.env.example` now lists every variable, with the required ones called out.

**Nothing deleted generated audio.** Every synthesis wrote a file to `storage/temp` and no
code removed any of them; the directory had reached 67 files when this was found. In
production that grows until the disk fills.

`storage.StartTempSweeper` runs hourly and on startup, removing files older than
`TEMP_AUDIO_RETENTION_HOURS` (default 24). The startup pass matters because a process killed
mid-run leaves behind files nothing else would ever claim. It only walks one level deep and
skips directories, so saved voices elsewhere in `storage/` are never touched — that
constraint is covered by a test, along with the boundary at the retention cutoff and a
missing directory being a no-op rather than an error.

Confirmed on the running deployment: the startup sweep removed 44 files and freed 9.3 MB,
leaving the 23 files newer than the retention window in place.

## Resolved: the manifest has exactly one definition

`core-tts/schemas.py` is the only place the manifest is *declared*. Everything downstream
merely describes its shape to decode it:

| Layer | Role |
|---|---|
| `core-tts/schemas.py` | **declares** the manifest (Pydantic defaults) |
| `core-backend/types/manifest.go` | structs to decode it, plus `ResolveCapabilities` |
| `frontend/src/lib/types.ts` | interfaces to read it, plus `lib/capabilities.ts` |

Neither the Go nor the TypeScript layer holds manifest *values* — only field definitions.
The backend caches what the engine returned and serves it back verbatim.

`frontend/scripts/prerender.js` used to break this, carrying an 89-line hand-written
FALLBACK_MANIFEST for build-time SSG when the backend was unreachable. It was a second
definition to keep in sync, and it silently went stale whenever the schema changed — which
is precisely what happened during the capability refactor.

It is gone. When the backend cannot be reached at build time the page is prerendered with
`initialManifest: null`: the shell renders, no `__SSG_MANIFEST__` global is injected, and
the client fetches the manifest on hydrate — the same path any cold client already takes.
Verified both ways: with the backend up, 7171 bytes of prerendered DOM and the manifest
inlined; with it down, 5866 bytes of shell, no global, CSS still inlined and JS still moved
to the end of body.

The trade is losing prerendered tab markup for builds made against a dead backend. That is
worth strictly less than a copy of the schema that no test would catch drifting.

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
