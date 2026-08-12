# Manifest Schema Review Notes

## Strengths

1. **Three-state boolean pattern is well-motivated.** `nil/undefined` = inherit, `true` = on, `false` = off. The single resolution function `resolveCapabilities()` in `lib/capabilities.ts` is the right place to centralize the Mode → Engine → Platform Default chain.

2. **AudioSpec appears at both engine and mode level** with the same resolution rule as capabilities. This was a clean addition.

3. **ChunkingSpec lives under `constraints`** rather than `audio_spec` or `ui_schema`. Correct — it governs what gets transmitted, not how anything looks.

4. **`ui_schema.input_panel` uses `ResolvedInputPanel`** for the resolved form, leaving the optional fields for the wire format. Good separation.

## Issues & Suggestions

### Cleaned Up (this branch)

- **`supports_ssml` removed from schema.** The field was declared in `EngineCapabilities` (Go + TypeScript) and resolved through `resolveCapabilities()`, but no frontend component ever read it — there was no SSML input toggle, no preview, no validation. Removed from: `EngineCapabilities`, `ResolvedCapabilities`, `PlatformDefaultCapabilities`, `resolveCapabilities()` (both Go and TS), shared fixture, and parity test.

- **`descriptions` field removed from `PresetVoiceSpec` and `VoiceOption`.** All voice metadata is now conveyed through `metadata: Record<string, string>`, which maps to the PostgreSQL JSONB column. The `descriptions` field was a duplicate shim added by an earlier AI pass and was never stored in the database. The frontend components had fallback logic (`descriptions || Object.values(metadata)`) that was dead code. See commit `fix/cache-cleanup`.

- **User cache in-memory RAM fallback removed.** The `userCache` previously maintained a parallel `entries map[string]userCacheEntry` that was written and read alongside Redis. This required dual-write, dual-delete, and lazy eviction, with no compensating benefit — a Redis flicker of a few seconds would just cause a few extra PostgreSQL SELECTs. The RAM fallback also meant that `InvalidateUser()` had to delete from two places, and with multiple replicas a stale entry on one replica would survive until the in-memory TTL expired. Now the cache is Redis-only; when Redis is unavailable, the cache degrades to a pass-through that queries PostgreSQL directly.

### Still Open

- **JSON Schema published at `docs/manifest.schema.json`.** AI engineers can validate their manifests with `jsonschema manifest.schema.json <your-manifest.json>`.

- **`Preset.speaker` removed.** The `Preset` interface had a `speaker` field that was always `String(v.name || '')` — a copy of `name` at the mapper level. No component ever read it. Removed the field and the mapper line.

## Priorities

All done! 🎉