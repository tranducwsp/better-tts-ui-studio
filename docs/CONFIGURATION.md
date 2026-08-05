# Configuration: what lives where

Every knob in this platform sits in one of three places. Which one is not arbitrary:

> **The engine decides it → manifest. The deployment decides it → environment variable.
> Nobody decides it at runtime → source constant.**

If you are adding a setting and cannot tell which bucket it belongs in, ask who would
change it. A different TTS engine? Manifest. The same engine deployed twice with different
limits? Environment. Neither? It is not configuration.

---

## 1. Manifest — declared by the engine

**File:** `core-tts/schemas.py` (Pydantic defaults). Served at `GET /info`, cached by the
backend, re-served at `GET /api/info`.

This is the only place manifest *values* exist. `core-backend/types/manifest.go` and
`frontend/src/lib/types.ts` describe the shape so each side can decode it; neither holds
defaults of its own beyond the platform fallbacks listed in §3.

| Want to change | Field |
|---|---|
| Which modes exist, and their order in the UI | `supported_modes`, `ui_schema.model_sort` |
| Whether a mode can clone / save voices / do pitch / emotion | `supported_modes[].capabilities` |
| Engine-wide capability defaults | `capabilities` |
| Max characters per request | `constraints.max_text_length` |
| How long text is cut into, and on what boundaries | `constraints.chunking` |
| Speed / pitch slider range | `constraints.speed_range`, `constraints.pitch_range` |
| Which emotions exist | `constraints.supported_emotions` |
| Output format, sample rate, downloadable formats | `audio_spec` |
| Max reference-audio upload size | `audio_spec.max_upload_bytes` |
| Seconds of reference audio the trimmer selects | `audio_spec.reference_audio_seconds` |
| Which widget draws a control | `ui_schema.option_panel[mode].*_type` |
| Preset voices shipped with the engine | `ui_schema.option_panel[mode].preset_voices` |
| Text cleaning rules | `ui_schema.input_panel.auto_format` |
| Fields on the "save voice" form | `ui_schema.option_panel[mode].voice_metadata_schema` |

Capability resolution is one rule: **mode value wins, else engine value, else platform
default.** `undefined` means inherit; `false` means this mode explicitly cannot. See
`docs/core_tts_protocol_spec.md` §2.1.

## 2. Environment — decided by the deployment

**Declared in:** `core-backend/config/settings.go` — one table, one row per variable, each
carrying its default, valid range, group and reason for existing.

**Read from:** `.env` (git-ignored) or `environment:` in `docker-compose.yml`.
**Template:** `.env.example`, *generated* from that table:

```
cd core-backend && go generate ./config
```

Do not edit `.env.example` by hand. `config.TestEnvExampleIsUpToDate` regenerates it into a
temp file and diffs, so a forgotten `go generate` fails the build rather than leaving the
template describing a configuration the backend no longer has. Defaults used to be written
three times — in the `getEnv` call, in the template, and in this document — with nothing
keeping them equal.

Adding a variable: add a row to `Settings`, read it in `LoadConfig` with `str()` or
`num()`, run `go generate`. The table is also the only place ranges live, so a value outside
one is rejected by name at startup.

Anything here is infrastructure: credentials, addresses, pool sizes, retention. An engine
never sees these values and must not depend on them.

Malformed values **stop the process at startup** rather than falling back silently — a
mistyped `DB_MAX_CONNS=twenty` used to be ignored, leaving the operator convinced a setting
had taken effect when it had not. The backend also logs the resolved configuration on boot
(secrets shown as state, never as values), so a misspelled *variable name* is visible too.

The table below is a summary; `settings.go` and `.env.example` are authoritative.

| Variable | Default | Notes |
|---|---|---|
| `SECRET_KEY` | **none — required** | Signs JWTs. Unset refuses to start (`requireAll`), rather than booting on a throwaway key. |
| `DEFAULT_ADMIN_USERNAME` / `_PASSWORD` | empty | Both blank ⇒ no admin seeded. |
| `DEFAULT_USER_USERNAME` / `_PASSWORD` | empty | Same. |
| `HOST` / `PORT` | `0.0.0.0` / `8000` | |
| `ACCESS_TOKEN_EXPIRE_MINUTES` | `10080` | 1 … 525600 |
| `DATABASE_URL` | local postgres | |
| `DB_MAX_CONNS` / `DB_MIN_CONNS` | `25` / `5` | min may not exceed max |
| `DB_MAX_CONN_LIFETIME_MINUTES` / `DB_MAX_CONN_IDLE_MINUTES` | `30` / `15` | |
| `DB_CONNECT_MAX_RETRIES` / `DB_CONNECT_RETRY_INTERVAL_SECONDS` | `10` / `2` | |
| `REDIS_URL` / `REDIS_PASSWORD` | `localhost:6379` / empty | Absent ⇒ in-memory mode |
| `CORE_ENGINE_URL` | `http://localhost:8001` | Where the manifest is fetched from. |
| `CORE_ENGINE_GRPC_URL` | `localhost:50051` | gRPC endpoint. Read into `Config`; the gRPC path is written but not wired in yet, so nothing consumes this today. Set it now and it will be correct when that path lands. |
| `FE_BUILDER_URL` | `http://frontend-builder:3001` | Backend → builder: signals a rebuild when the manifest changes. Carries no payload. |
| `VITE_BACKEND_URL` | `http://core-backend:8000` | Builder → backend: where it fetches the manifest. Read by the frontend, not the backend. |
| `TTS_CLIENT_TIMEOUT_SECONDS` | `60` | |
| `COOKIE_SECURE` | `1` | Sets Secure on the session cookie. Use `0` only for local http://localhost. |
| `POSTGRES_PASSWORD` | **none — required** | Read by docker-compose to build `DATABASE_URL`; compose refuses to start while empty. |
| `STORAGE_DIR` | `storage` | |
| `TEMP_AUDIO_RETENTION_HOURS` | `24` | Sweeper deletes older temp audio hourly |
| `MAX_UPLOAD_SIZE_MB` | `256` | Infrastructure ceiling; `audio_spec.max_upload_bytes` can lower it |
| `CORS_ALLOWED_ORIGINS` | localhost dev ports | Comma-separated |
| `UI_MODE` | `beauty` | Read by the *engine*, surfaced as `ui_schema.ui_mode` |

## 3. Source constants — platform fallbacks

Used only when no manifest has loaded yet: during SSG against an offline backend, or in the
moment before the first `/api/info` response arrives.

| Constant | Where | Value |
|---|---|---|
| Capability defaults | `types.PlatformDefaultCapabilities` (Go) and `PLATFORM_DEFAULTS` (TS) | preset_voices / streaming / speed `true`, rest `false` |
| Text length | `types.DefaultMaxTextLength` (Go) and `DEFAULT_TEXT_LIMIT` (TS) | 3000 |
| Speed range | `DEFAULT_SPEED_RANGE` (`lib/ranges.ts`) | 0.5 … 2.0, step 0.1 |
| Pitch range | `DEFAULT_PITCH_RANGE` (`lib/ranges.ts`) | -10 … 10, step 0.5 |
| Audio spec | `lib/audioSpec.ts` | WAV, 10 MB upload, 5 s reference clip |

Every one of these is reached through a resolver rather than read from the manifest at the
point of use. Reading `manifest?.constraints?...` directly in a component is how the same
fallback ends up written twice with different values.

These exist twice because the two runtimes cannot share code. They are pinned together by
`docs/capability-resolution-cases.json`, which both `core-backend/tests/parity_test.go` and
`frontend/src/lib/capabilities.test.ts` read: change one side's constant and the other
side's test fails. Add cases to that file rather than to either test.

---

## Known wrinkles

One setting sits on the wrong side of the line. Documented rather than silently
inconsistent:

*(`MAX_UPLOAD_SIZE_MB` used to be listed here as belonging in the manifest. The engine now
declares `audio_spec.max_upload_bytes` and the stricter of the two wins — the env var is the
infrastructure ceiling, the manifest value is what the engine can actually process.)*

*(`VITE_BACKEND_URL` is listed in `settings.go` despite the backend never reading it. It is
the other half of the FE_BUILDER_URL link, and splitting the two across separate files would
mean remembering two places to change one connection. `config.TestReadByVarsAreNotLoaded`
keeps the boundary honest: a variable marked `ReadBy` must not be read by `LoadConfig`.)*

*(`UI_MODE` used to be one of these — the engine published it as `ui_schema.ui_mode` while
the frontend build separately honoured `VITE_UI_MODE`. The build variable is gone; the
engine is the only source now. See below.)*

---

## How `UI_MODE` reaches the browser

Worth spelling out because it is the one setting that travels the whole way round, and it
shows what "the engine decides" means in practice:

```
UI_MODE=fast on the engine
  → engine publishes ui_schema.ui_mode = "fast" in its manifest
  → POST /api/internal/engine/reload refreshes the backend cache
  → backend fires FE_BUILDER_URL/rebuild
  → frontend-builder runs `npm run build`
  → prerender.js reads manifest.ui_schema.ui_mode and strips icons, webfonts
    and GPU filters from the bundle
  → App.svelte sets data-mode on <body> at runtime from the same field
```

Measured end to end: switching the engine to `fast` and calling the reload webhook took the
prerendered page from 50007 to 30327 bytes with FontAwesome markup and woff2 preloads gone;
switching back restored both.
