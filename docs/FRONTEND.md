# 🎨 Frontend Technical Specification

This document details the technical specification of the **Frontend Web Studio** application, including interface architecture, technologies used, the automatic UI generation mechanism from the Manifest (Dynamic Schema-Driven Rendering), real-time audio processing with the Web Audio API, and SSE Streaming.

---

## 🛠️ 1. Technologies Used (Tech Stack)

| Component | Technology / Library | Reason for Selection |
| :--- | :--- | :--- |
| **Core Framework** | **Svelte 5 (Runes)** | Uses fine-grained reactivity with `$state`, `$derived`, `$effect`, no Virtual DOM, providing high performance and extremely small bundle sizes |
| **Build Tool** | **Vite 5** | High-speed bundler, instant HMR (Hot Module Replacement) support |
| **Language** | **TypeScript** | Type-safety for all Manifest data, API payloads, and Web Audio buffers |
| **Styling Engine** | **Vanilla CSS (Glassmorphism)** | Custom-defined CSS Variables, modern glassmorphism effects, optimized Dark Mode, no Tailwind |
| **Icons & Fonts** | **FontAwesome 6 + Inter Font** | Standardized Inter font and rich vector icon system |
| **Audio Engine** | **Web Audio API** | Decode binary PCM/WAV directly in the browser, concatenate audio segments for real-time playback |

---

## 🏛️ 2. Component Architecture

The interface is designed with a centralized component model, divided by responsibility:

```
App.svelte (Root State & View Manager)
 ├── Header.svelte (Nav, Model Selector, Manifest Reload, User Profile)
 ├── AuthModal.svelte (Login / Register Modal)
 ├── AdminModal.svelte (Admin User Approval Modal)
 └── Studio Main Container
      ├── TextInputPanel.svelte (Text Input, Regex Rules, Find/Replace, Extract File)
      ├── GenericEnginePanel.svelte (Auto-generated Sliders, Options from Manifest UI Schema)
      │    └── VoiceSelect.svelte (Select system voices & personal Clone voices)
      │         └── CreateVoiceModal.svelte (Create new Clone voice)
      │              └── WaveformTrimmer.svelte (View waveform & Trim sample audio)
      ├── StreamingPanel.svelte (Realtime Chunk Audio Playback & SSE Progress)
      └── HistoryModal.svelte (View audio synthesis history)
```

### 2.1. Main Component Roles in Detail

1. **`App.svelte`**:
   - Manage personal login state (read via `/api/me`), load Engine Manifest information (`/api/info`).
   - Manage display Modals (Auth, History, Admin, Create Voice).

2. **`GenericEnginePanel.svelte`**:
   - **Core Schema-Driven UI Component**: Parse the `ui_schema.components` array from the Manifest to automatically render:
     - Component `slider`: Render HTML5 `<input type="range">` slider with step, min/max values, and real-time metrics.
     - Component `select`: Render mode/preset selection dropdown.
     - Component `toggle`: Render on/off switch.
     - Component `notice_banners`: Display visual notice banners from the AI Engineer.

3. **`TextInputPanel.svelte`**:
   - Multi-line text input frame with real-time word and character count.
   - Integrated Search & Replace tool for convenient editing.
   - Allows uploading document files (PDF, DOCX, TXT) to automatically extract text via the `/api/extract-text` API.
   - Apply **Regex Auto-Formatting Engine** rules (declared from the Manifest) to clean text before sending for synthesis.

4. **`StreamingPanel.svelte` & Web Audio Engine**:
   - Manage SSE Stream connection (`/api/stream/tasks/{task_id}`) to receive per-chunk progress notifications.
   - Receive Chunk completion signal -> Call API to get Binary Audio -> Use Web Audio API `AudioContext.decodeAudioData()` to decode and concatenate seamless audio playback.

5. **`WaveformTrimmer.svelte`**:
   - Custom component using HTML5 Canvas to draw the Audio Waveform of the uploaded sample file.
   - Allows users to drag and drop start/end time markers to trim the best quality audio segment for Voice Cloning.

---

## ⚡ 3. State Management Mechanism Using Svelte 5 Runes

Svelte 5 replaces the old `$:` mechanism with a powerful **Runes** system:

- **`$state()`**: Declare reactive state.
  ```typescript
  let manifest = $state<EngineManifest | null>(null);
  let isSynthesizing = $state(false);
  let params = $state<Record<string, any>>({});
  ```
- **`$derived()`**: Automatically compute dependent values.
  ```typescript
  let cleanText = $derived(applyRegexRules(rawText, manifest?.regex_rules));
  let charCount = $derived(rawText.length);
  ```
- **`$effect()`**: Listen to state changes to execute side-effects (send API, update UI canvas).
  ```typescript
  $effect(() => {
    if (selectedModelId) {
      loadModelVoices(selectedModelId);
    }
  });
  ```

---

## 🔊 4. Binary Audio Processing & Streaming

### 4.1. Binary Chunk Audio Decoding Flow

```
 [Binary ArrayBuffer] ──> AudioContext.decodeAudioData() ──> [AudioBuffer]
                                                                  │
 [Web Audio API Node Pipeline] ◄──────────────────────────────────┘
   AudioBufferSourceNode ──> GainNode (Volume) ──> AudioDestinationNode (Speakers)
```

1. When each audio Chunk completes, the Frontend downloads the data as an `ArrayBuffer`.
2. The browser decodes it into a raw `AudioBuffer` via `AudioContext`.
3. The audio segment is played consecutively and displayed with a visual progress bar on the `StreamingPanel`.

---

## 🏗️ 5. Prerendering Process & Builder Server

The Frontend integrates a small Node.js server (`scripts/builder_server.js`):

- **Task**: Listen for HTTP POST Webhook requests from the Backend Gateway (`FE_BUILDER_URL/reload`).
- **Action**: When receiving the reload signal, the process calls the `prerender.js` script to connect to the Backend, fetch the latest `GET /api/info` information, and rebuild the static HTML Bundle file (`dist/index.html`).
- Thanks to this mechanism, SEO, Title, Meta Descriptions, and initial configuration tags are always accurate with the current Manifest without slowing down the user's page load time.