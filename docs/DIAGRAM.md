# System Architecture Diagrams

> Mermaid diagrams describing the major flows and structures of the AI Voice Studio platform.

---

## 1. System Architecture

```mermaid
graph TB
    subgraph Browser["Browser (Svelte 5)"]
        UI[App UI]
        SW[Service Worker]
    end

    subgraph Gateway["Go Gateway (API)"]
        AUTH[Auth Middleware]
        RATE[Rate Limiter]
        MANIFEST[Manifest Cache]
        TASK[Task Manager]
        UPLOAD[Upload Handler]
    end

    subgraph Redis["Redis (Shared State)"]
        TASK_STATE[task:{id} — state & progress]
        AUDIO[audio:{id} — output blob]
        QUEUE[queue:{engine} — pending tasks]
        RATELIMIT[ratelimit:{user} — counters]
        USER_CACHE[auth:user:{name} — 5s TTL]
        ONLINE[online:{user} — 60s heartbeat]
        PUBSUB[Pub/Sub — task events]
    end

    subgraph PG["PostgreSQL (Durable Data)"]
        USERS[users + auth]
        HISTORY[synthesis history]
        VOICES[voice metadata]
    end

    subgraph Worker["Engine Worker (Python)"]
        ENGINE[Core AI Engine]
    end

    Browser -->|HTTP/SSE| Gateway
    Gateway -->|read/write| Redis
    Gateway -->|read| PG
    Gateway -->|gRPC| Worker
    Worker -->|write progress| Redis
    Worker -->|write output| Redis
    Worker -->|notify| PUBSUB
    Gateway -->|subscribe| PUBSUB
```

---

## 2. Manifest Resolution Chain

```mermaid
flowchart LR
    subgraph Engine["Engine Manifest"]
        EC[EngineCapabilities<br/>engine-wide defaults]
        EAS[AudioSpec<br/>engine-wide defaults]
    end

    subgraph Mode["Per-Mode Override"]
        MC[Mode.Capabilities<br/>nil = inherit]
        MAS[Mode.AudioSpec<br/>nil = inherit]
    end

    subgraph Platform["Platform Defaults"]
        PC[PlatformDefaultCapabilities<br/>hardcoded fallback]
        PAS[PlatformDefaultAudioSpec<br/>hardcoded fallback]
    end

    subgraph Resolver["resolveCapabilities() / ResolveAudioSpec()"]
        CAP[ResolvedCapabilities<br/>Mode → Engine → Platform]
        AS[ResolvedAudioSpec<br/>Mode → Engine → Platform]
    end

    MC -->|"stated? → use"| CAP
    EC -->|"mode nil? → use"| CAP
    PC -->|"engine nil? → use"| CAP

    MAS -->|"stated? → use"| AS
    EAS -->|"mode nil? → use"| AS
    PAS -->|"engine nil? → use"| AS
```

---

## 3. Auth State Machine

```mermaid
stateDiagram-v2
    [*] --> Loading: app starts

    Loading --> Auth: checkCurrentUser() → null
    Loading --> App: checkCurrentUser() → user

    Auth --> App: login/register success
    App --> Auth: logout()

    state Auth {
        Login --> Login: wrong password → inline error
        Login --> Register: switch mode
        Register --> Login: switch mode
        Register --> Login: registration success
    }

    state App {
        [*] --> Idle
        Idle --> Streaming: job starts
        Streaming --> Idle: job ends / close
    }
```

---

## 4. Synthesis Request Flow

```mermaid
sequenceDiagram
    participant B as Browser
    participant G as Gateway
    participant R as Redis
    participant W as Worker
    participant P as PostgreSQL

    B->>G: POST /api/synthesize
    G->>R: rate-limit check
    G->>R: user cache lookup
    R-->>G: user data
    G->>P: (if cache miss) user lookup
    G->>R: LPUSH queue:{engine} task_id
    G-->>B: {task_id, status: queued}

    B->>G: GET /api/task/{id} (poll)
    G->>R: GET task:{id}

    R-->>W: BRPOP queue:{engine}
    W->>R: SET task:{id} → processing
    W->>R: PUBLISH task:{id} → processing
    W->>W: synthesize audio
    W->>R: SET task:{id} → completed
    W->>R: SET audio:{id} → blob
    W->>R: PUBLISH task:{id} → completed

    R-->>G: (pub/sub notification)
    G-->>B: SSE: task completed

    B->>G: GET /api/audio/{id}
    G->>R: GET audio:{id}
    R-->>G: blob
    G-->>B: audio data
```

---

## 5. Cache Architecture

```mermaid
flowchart TD
    subgraph Redis["Redis — cross-process state"]
        TASK[task:{id} — TTL 5min]
        AUDIO[audio:{id} — TTL 5min]
        QUEUE[queue:{engine} — list]
        RATE[ratelimit:{user} — window]
        UCACHE[auth:user:{name} — TTL 5s]
        ONLINE[online:{user} — TTL 60s]
    end

    subgraph RAM["In-Memory — hot singletons"]
        MANIFEST[ManifestCache]
        TASKMGR[TaskManager]
        VOICE_CACHE[PresetVoiceCache]
        CONCURRENCY[Concurrency limiter]
    end

    subgraph DB["PostgreSQL — durable state"]
        USERS[users]
        HISTORY[history]
        VOICES[voices]
    end

    subgraph CLIENT["Browser"]
        FE[Frontend]
    end

    FE -->|reads| MANIFEST
    FE -->|API calls| TASK
    FE -->|API calls| AUDIO

    API[Gateway API] -->|reads/writes| TASK
    API -->|reads/writes| AUDIO
    API -->|reads/writes| QUEUE
    API -->|reads/writes| RATE
    API -->|reads| UCACHE
    API -->|miss →| DB
    API -->|reads| MANIFEST
    API -->|updates| ONLINE

    WORKER[Worker] -->|reads/writes| TASK
    WORKER -->|writes| AUDIO
    WORKER -->|reads| QUEUE
```

---

## 6. File & Module Dependencies

```mermaid
graph TD
    subgraph Frontend["Frontend (TypeScript / Svelte 5)"]
        APP[App.svelte]
        AUTH[AuthModal.svelte]
        HEADER[Header.svelte]
        TEXT[TextInputPanel.svelte]
        ENGINE[GenericEnginePanel.svelte]
        STREAM[StreamingPanel.svelte]
        VOICE[VoiceSelect.svelte]
        HISTORY[HistoryModal.svelte]
        ADMIN[AdminModal.svelte]

        API[lib/api.ts]
        TYPES[lib/types.ts]
        CAP[lib/capabilities.ts]
        AUDIO_SPEC[lib/audioSpec.ts]
        INPUT_PANEL[lib/inputPanel.ts]
        TEXT_LIMITS[lib/textLimits.ts]
        TOAST[lib/toast.svelte.ts]
    end

    subgraph Backend["Backend (Go)"]
        MANIFEST[types/manifest.go]
        VALIDATE[types/validate.go]
        HANDLERS[handlers/*.go]
        AUTH_MID[middleware/auth.go]
        UCACHE[middleware/user_cache.go]
        RATELIMIT[middleware/ratelimit.go]
        STATE[state/*.go]
        SYNTH[synth/pipeline.go]
    end

    APP --> AUTH
    APP --> HEADER
    APP --> TEXT
    APP --> ENGINE
    APP --> STREAM
    APP --> HISTORY
    APP --> ADMIN
    APP --> API
    APP --> TOAST

    ENGINE --> VOICE
    ENGINE --> CAP
    ENGINE --> AUDIO_SPEC
    ENGINE --> API

    TEXT --> INPUT_PANEL
    TEXT --> TEXT_LIMITS

    API --> TYPES
    CAP --> TYPES
    AUDIO_SPEC --> TYPES
    INPUT_PANEL --> TYPES

    HANDLERS --> MANIFEST
    HANDLERS --> VALIDATE
    HANDLERS --> STATE
    HANDLERS --> AUTH_MID
    AUTH_MID --> UCACHE
    SYNTH --> MANIFEST
```

---

## 7. Three-State Boolean Pattern

```mermaid
flowchart LR
    subgraph Engine["Engine declares"]
        E_TRUE["true — explicitly ON"]
        E_FALSE["false — explicitly OFF"]
        E_NIL[nil — not stated]
    end

    subgraph Mode["Mode declares"]
        M_TRUE["true — override ON"]
        M_FALSE["false — override OFF"]
        M_NIL[nil — inherit]
    end

    subgraph Result["Resolved"]
        R_ON["true"]
        R_OFF["false"]
    end

    M_TRUE --> R_ON
    M_FALSE --> R_OFF
    M_NIL --> E_TRUE --> R_ON
    M_NIL --> E_FALSE --> R_OFF
    M_NIL --> E_NIL --> R_ON["platform default"]
    M_NIL --> E_NIL --> R_OFF["platform default"]
```

---

## 8. Test Coverage Map

```mermaid
graph LR
    subgraph Go_Tests["Go Backend Tests"]
        RESOLVE[resolve_test.go]
        VALIDATE_T[validate_test.go]
        AUDIO_T[audio_spec_test.go]
        PARITY[contract/parity/resolver_test.go]
    end

    subgraph TS_Tests["Frontend Tests"]
        CAP_T[capabilities.test.ts]
        AUDIO_SPEC_T[audioSpec.test.ts]
        RANGES[ranges.test.ts]
    end

    subgraph Fixture["Shared Fixture"]
        FIXTURE[capability-resolution-cases.json]
    end

    RESOLVE -->|ResolveCapabilities| FIXTURE
    PARITY -->|ResolveCapabilities| FIXTURE
    CAP_T -->|resolveCapabilities| FIXTURE

    AUDIO_T -->|ResolveAudioSpec| MANIFEST[manifest.go]
    AUDIO_SPEC_T -->|resolveAudioSpec| AUDIO_SPEC[audioSpec.ts]
    VALIDATE_T -->|Validate| VALIDATE[validate.go]
    RANGES -->|format ranges| TEXT_LIMITS[textLimits.ts]
```

---

## Legend

| Symbol | Meaning |
|--------|---------|
| `→` | Data flow / dependency |
| `-->>` | Async / poll / subscribe |
| `nil` | Not stated, inherit from parent |
| Platform Default | Hardcoded fallback when nothing else declares a value |