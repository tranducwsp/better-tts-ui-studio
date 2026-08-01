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
| Which widget draws a control | `ui_schema.option_panel[mode].*_type` |
| Preset voices shipped with the engine | `ui_schema.option_panel[mode].preset_voices` |
| Text cleaning rules | `ui_schema.input_panel.auto_format` |
| Fields on the "save voice" form | `ui_schema.option_panel[mode].voice_metadata_schema` |

Capability resolution is one rule: **mode value wins, else engine value, else platform
default.** `undefined` means inherit; `false` means this mode explicitly cannot. See
`docs/core_tts_protocol_spec.md` §2.1.

## 2. Environment — decided by the deployment

**File:** `.env` (git-ignored), or `environment:` in `docker-compose.yml`. Template:
`.env.example`.

Anything here is infrastructure: credentials, addresses, pool sizes, retention. An engine
never sees these values and must not depend on them.

Malformed values **stop the process at startup** rather than falling back silently — a
mistyped `DB_MAX_CONNS=twenty` used to be ignored, leaving the operator convinced a setting
had taken effect when it had not. The backend also logs the resolved configuration on boot
(secrets shown as state, never as values), so a misspelled *variable name* is visible too.

| Variable | Default | Notes |
|---|---|---|
| `SECRET_KEY` | *random per boot* | Signs JWTs. Unset logs a warning; sessions then die on restart and replicas cannot share them. |
| `DEFAULT_ADMIN_USERNAME` / `_PASSWORD` | empty | Both blank ⇒ no admin seeded. |
| `DEFAULT_USER_USERNAME` / `_PASSWORD` | empty | Same. |
| `HOST` / `PORT` | `0.0.0.0` / `8000` | |
| `ACCESS_TOKEN_EXPIRE_MINUTES` | `10080` | 1 … 525600 |
| `DATABASE_URL` | local postgres | |
| `DB_MAX_CONNS` / `DB_MIN_CONNS` | `25` / `5` | min may not exceed max |
| `DB_MAX_CONN_LIFETIME_MINUTES` / `DB_MAX_CONN_IDLE_MINUTES` | `30` / `15` | |
| `DB_CONNECT_MAX_RETRIES` / `DB_CONNECT_RETRY_INTERVAL_SECONDS` | `10` / `2` | |
| `REDIS_URL` / `REDIS_PASSWORD` | `localhost:6379` / empty | Absent ⇒ in-memory mode |
| `CORE_ENGINE_URL` | `http://localhost:8001` | Where the manifest is fetched from. Legacy alias: `CORE_TTS_URL`. |
| `CORE_ENGINE_GRPC_URL` | `localhost:50051` | gRPC endpoint. Legacy alias: `CORE_TTS_GRPC_URL`. |
| `FE_BUILDER_URL` | `http://frontend-builder:3001` | Notified to rebuild the SSG bundle when the manifest changes. |
| `TTS_CLIENT_TIMEOUT_SECONDS` | `60` | |
| `STORAGE_DIR` | `storage` | |
| `TEMP_AUDIO_RETENTION_HOURS` | `24` | Sweeper deletes older temp audio hourly |
| `MAX_UPLOAD_SIZE_MB` | `32` | See "known wrinkles" below |
| `CORS_ALLOWED_ORIGINS` | localhost dev ports | Comma-separated |
| `UI_MODE` | `beauty` | Read by the *engine*, surfaced as `ui_schema.ui_mode` |

## 3. Source constants — platform fallbacks

Used only when no manifest has loaded yet: during SSG against an offline backend, or in the
moment before the first `/api/info` response arrives.

| Constant | Where | Value |
|---|---|---|
| Capability defaults | `types.PlatformDefaultCapabilities` (Go) and `PLATFORM_DEFAULTS` (TS) | preset_voices / streaming / speed `true`, rest `false` |
| Text length | `types.DefaultMaxTextLength` (Go) and `DEFAULT_TEXT_LIMIT` (TS) | 3000 |

These exist twice because the two runtimes cannot share code. They are pinned together by
`docs/capability-resolution-cases.json`, which both `core-backend/tests/parity_test.go` and
`frontend/src/lib/capabilities.test.ts` read: change one side's constant and the other
side's test fails. Add cases to that file rather than to either test.

---

## Known wrinkles

Two settings currently sit on the wrong side of the line. Documented rather than silently
inconsistent:

**`MAX_UPLOAD_SIZE_MB` is an env var but should be manifest.** How large a reference audio
clip may be is a property of the engine that consumes it, not of the deployment. The UI
separately hardcodes "Max 10MB" in its dropzone label, which agrees with neither. Fixing it
means adding `audio_spec.max_upload_bytes` — see `PLATFORM_GAPS.md` §2.

**`UI_MODE` is reachable by two routes.** The engine reads it from its own environment and
publishes it as `ui_schema.ui_mode`; the frontend build also honours `VITE_UI_MODE`. The
manifest value wins when present, so set it on the engine and leave the build variable
alone.
