# Manifest examples

Real `GET /api/info` responses, captured from running engines. Use them as a starting point
rather than transcribing the schema by hand.

| File | Engine | What it shows |
|---|---|---|
| `manifest-engine.json` | `core-tts-example` (the bundled example engine) | The common case: three modes, cloning on one of them, pitch and emotion off everywhere. |

## Reading `manifest-engine.json`

Two things in it are worth pointing at, because they are the parts most easily got wrong.

**Per-mode capabilities state only what differs.** The engine-wide `capabilities` block sets
the baseline, and each mode overrides from there:

```jsonc
"capabilities": {          // engine-wide baseline
  "supports_cloning": false,
  "supports_voice_saving": false,
  ...
},
"supported_modes": [
  { "id": "clone",
    "capabilities": { "supports_cloning": true, "supports_voice_saving": true } }
]
```

`standard` and `fast` repeat `false` here for readability, but they could have omitted the
block entirely and inherited the same result. What they could *not* do is omit it and expect
`true` — absent means inherit, not enable. See `core_tts_protocol_spec.md` §2.1.

**`constraints.chunking.delimiters` are zero-width lookbehinds.** They mark a cut point
without consuming characters, because the platform applies them with `split()`:

```json
["(?<=\\.\\s*\\n)", "(?<=[.!?]\\s+)"]
```

A pattern that matches content instead — `[^.!?]+[.!?]+`, say — still runs, but a split
discards whatever it matched, so text goes missing. Prefer lookbehind.

## Regenerating

```
curl -s http://localhost:8000/api/info | python3 -m json.tool > docs/examples/manifest-engine.json
```

Worth redoing after any change to `core-tts-example/schemas.py`, so the example keeps matching what
the engine actually serves.
