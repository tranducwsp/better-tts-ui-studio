<script lang="ts">
  import type { VoiceOption, UniversalManifest, EngineModeSpec, Preset, JobDetailResponse } from '../types';
  import VoiceSelect from './VoiceSelect.svelte';
  import WaveformTrimmer from './WaveformTrimmer.svelte';
  import CreateVoiceModal from './CreateVoiceModal.svelte';
  import { fetchVoices, fetchPresets, cloneVoiceTemp, deleteCloneVoice } from '../api';
  import { acceptedUploadFormats, maxUploadBytes, maxUploadLabel, referenceSizeError, supportedFormatsLabel } from '../audioSpec';
  import { resolveCapabilities } from '../capabilities';
  import { pitchRange, speedRange } from '../ranges';
  import { resolveStreamingThreshold } from '../textLimits';
  import { toast } from '../toast.svelte';

  interface Props {
    text: string;
    activeMode: EngineModeSpec;
    manifest: UniversalManifest | null;
    reloadedJob?: JobDetailResponse | null;
    /**
     * Asks the parent to open the streaming panel with these settings. The panel is a
     * sibling of this one rather than a child, so a long job is not nested inside the
     * settings card it was started from.
     */
    onStartStreaming?: (params: {
      engine: string;
      voice: string;
      speed: number;
      pitch?: number;
      emotion?: string;
    }) => void;
  }

  let { text, activeMode, manifest, reloadedJob = null, onStartStreaming }: Props = $props();

  // Mode option spec derived from manifest ui_schema
  let modelOption = $derived(manifest?.ui_schema?.option_panel?.[activeMode.id] || null);

  // Every capability question goes through one resolver, so the mode-over-engine
  // precedence rule exists in a single place rather than as ?? chains here.
  let caps = $derived(resolveCapabilities(manifest, activeMode));

  let supportsCloning = $derived(caps.supports_cloning);
  let supportsVoiceSaving = $derived(caps.supports_voice_saving);
  let supportsPresetVoices = $derived(caps.supports_preset_voices);
  let supportsStreaming = $derived(caps.supports_streaming);
  let supportsSpeed = $derived(caps.supports_speed);
  let supportsPitch = $derived(caps.supports_pitch);
  // An engine claiming emotion support without a vocabulary has nothing to offer.
  let supportsEmotion = $derived(
    caps.supports_emotion && (manifest?.constraints?.supported_emotions?.length || 0) > 0
  );

  let availableEmotions = $derived(manifest?.constraints?.supported_emotions ?? []);

  // Slider bounds resolved once, so the slider and number variants of each control can
  // never disagree about min/max/step.
  let speedBounds = $derived(speedRange(manifest));
  let pitchBounds = $derived(pitchRange(manifest));

  let modeVoices = $state<VoiceOption[]>([]);

  // Active voice list: merge preset_voices from manifest (SSG-sync) with modeVoices
  // (async-loaded user clone voices). Preset voices are available immediately during SSR;
  // user clone voices are fetched after hydration and prepended before presets.
  // Map PresetVoiceSpec → VoiceOption to fix sample_url (snake_case) → sampleUrl (camelCase).
  let activeVoices = $derived.by(() => {
    // 1. Preset voices from manifest (available immediately during SSR — no async fetch needed)
    const preset: VoiceOption[] = (modelOption?.preset_voices || []).map((pv) => ({
      id: pv.id,
      name: pv.name,
      metadata: pv.metadata,
      sampleUrl: pv.sample_url,
    }));

    // 2. If preset_voices exist, merge with user clone voices (modeVoices, loaded async)
    if (preset.length > 0) {
      // User clone voices (loaded async, deletable=true)
      const user = modeVoices.filter((v) => v.deletable);
      // Merge: user voices first, then preset_voices, deduplicate by id/name
      const combined = [...user, ...preset];
      const seen = new Set<string>();
      return combined.filter((v) => {
        const key = v.id || v.name;
        if (key && !seen.has(key)) {
          seen.add(key);
          return true;
        }
        return false;
      });
    }

    // 3. No preset_voices: use modeVoices (from API fallback or user presets)
    return modeVoices;
  });


  // States
  let selectedVoice = $state('');
  let speed = $state(1.0);
  let pitch = $state(0.0);
  let selectedEmotion = $state('');
  let referenceAudioPath = $state('');

  let isCloningTemp = $state(false);

  // Synthesize button text/icon per mode — matches old core-tts UI personality.
  let synthLabel = $derived.by(() => {
    const id = activeMode.id;
    if (id === 'fast' || id === 'express') return { text: 'Express Synthesis', icon: 'fa-bolt' };
    if (id === 'zero_shot_clone' || id === 'clone') return { text: 'Synthesize Audio', icon: 'fa-play' };
    return { text: 'Synthesize Audio', icon: 'fa-play' };
  });
  let isCreateModalOpen = $state(false);
  let selectedFile = $state<File | null>(null);


  // Hands the current settings to the parent, which owns the streaming panel.
  function requestStreaming() {
    onStartStreaming?.({
      engine: activeMode.id,
      voice: referenceAudioPath || selectedVoice,
      speed,
      pitch: supportsPitch ? pitch : undefined,
      emotion: supportsEmotion ? selectedEmotion : undefined,
    });
  }

  // Load engine voices & user custom saved clone voices from DB
  //
  // Guard with a counter: each call increments generation, after fetch completes check if generation
  // still matches — if the tab changed mid-flight, discard the stale result. AbortController works too
  // but requires passing signal through both fetchVoices/fetchPresets; a counter only needs one variable.
  let voiceLoadGeneration = 0;

  async function loadVoicesForMode(modeId: string) {
    const gen = ++voiceLoadGeneration;
    modeVoices = [];
    try {
      let combined: VoiceOption[] = [];
      const currentOption = manifest?.ui_schema?.option_panel?.[modeId] || null;

      // Only fetch from API if there are no preset_voices in the manifest.
      // preset_voices are handled synchronously by the activeVoices derivation so no duplication needed here.
      if (!currentOption?.preset_voices || currentOption.preset_voices.length === 0) {
        const engineVoices = await fetchVoices(modeId);
        if (engineVoices && engineVoices.length > 0) {
          combined = [...engineVoices];
        }
      }

      // Fetch user clone voices if mode supports voice saving
      if (resolveCapabilities(manifest, modeId).supports_voice_saving) {
        const userPresets = await fetchPresets(modeId);
        if (userPresets && userPresets.length > 0) {
          const userVoices: VoiceOption[] = userPresets.map((p) => ({
            id: p.id,
            name: p.name,
            deletable: true,
            metadata: p.metadata,
          }));
          combined = [...userVoices, ...combined];
        }
      }

      // Deduplicate voices by id/name
      const uniqueVoices: VoiceOption[] = [];
      const seenKeys = new Set<string>();
      for (const v of combined) {
        const key = v.id || v.name;
        if (key && !seenKeys.has(key)) {
          seenKeys.add(key);
          uniqueVoices.push(v);
        }
      }

      // Mode changed mid-flight — the old fetch returns later but does not overwrite.
      if (gen !== voiceLoadGeneration) return;

      modeVoices = uniqueVoices;
      if (uniqueVoices.length > 0 && (!selectedVoice || !uniqueVoices.some(v => (v.id || v.name) === selectedVoice))) {
        selectedVoice = uniqueVoices[0].id || uniqueVoices[0].name;
      }
    } catch (err) {
      console.error('Error loading voices for mode:', err);
    }
  }

  // Plain (non-reactive) guard: this is written by the effect that reads it, so it must
  // not be $state or the effect would depend on its own write.
  let lastModeId = '';

  $effect(() => {
    if (activeMode && activeMode.id && activeMode.id !== lastModeId) {
      lastModeId = activeMode.id;
      speed = speedBounds.default;
      pitch = pitchBounds.default;
      // Emotion vocabularies are per-engine; a value carried over from another mode
      // may not exist in this one, so always reset it.
      selectedEmotion = '';
      selectedVoice = '';
      // Reference audio belongs to a specific engine; stale path would be sent to
      // the wrong engine on the next synthesis request.
      selectedFile = null;
      referenceAudioPath = '';
      loadVoicesForMode(activeMode.id);
    }
  });

  // Handle reloaded job state
  $effect(() => {
    if (reloadedJob && reloadedJob.engine === activeMode.id) {
      if (reloadedJob.voice) selectedVoice = reloadedJob.voice;
      if (reloadedJob.speed) speed = reloadedJob.speed;
      // Absent means the engine had no such control for that job; keep the mode default.
      // Compared against undefined so a stored pitch of 0 still restores.
      if (reloadedJob.pitch !== undefined) pitch = reloadedJob.pitch;
      if (reloadedJob.emotion) selectedEmotion = reloadedJob.emotion;
      if (reloadedJob.chunks && reloadedJob.chunks.length > 0) {
        requestStreaming();
      }
    }
  });

  // Auto-select the first voice when the voice list changes (preset_voices from SSR
  // or after user voices load async). Prevents the case where selectedVoice is empty
  // while activeVoices already has data.
  $effect(() => {
    if (activeVoices.length > 0 && !selectedVoice) {
      selectedVoice = activeVoices[0].id || activeVoices[0].name;
    }
  });

  async function handleFileUpload(e: Event) {
    const target = e.target as HTMLInputElement;
    if (!target.files?.length) return;
    const file = target.files[0];

    // Check here as well as on the server: rejecting a 200 MB file after uploading it
    // wastes the user's bandwidth to reach the same answer.
    const ceiling = maxUploadBytes(manifest, activeMode);
    if (file.size > ceiling) {
      toast.show(`File is ${(file.size / 1048576).toFixed(1)} MB; the engine accepts up to ${maxUploadLabel(manifest, activeMode)}.`, 'error');
      target.value = '';
      return;
    }

    selectedFile = file;
    isCloningTemp = true;
    toast.show('Processing reference audio file...', 'info');
    try {
      const tempPath = await sendReference(file);
      referenceAudioPath = tempPath;
      toast.show('Reference audio file loaded successfully!', 'success');
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.show('Error loading reference file: ' + errMsg, 'error');
    } finally {
      isCloningTemp = false;
      target.value = '';
    }
  }

  // Both reference uploads go through here so the engine's clip ceiling is enforced once.
  // The trimmer path needs it most: trimming is exactly what a user does to get under the
  // limit, so failing there must say so rather than surface a generic upload error.
  async function sendReference(file: File): Promise<string> {
    const tooBig = referenceSizeError(file.size, manifest, activeMode);
    if (tooBig) throw new Error(tooBig);
    return cloneVoiceTemp(file, activeMode.id);
  }

  // Deleting a saved voice needs a confirm: the reference clip goes with it on the server,
  // so there is nothing to undo from. Only voices marked deletable reach here — engine
  // presets share the list but have no route to delete.
  async function handleDeleteVoice(v: VoiceOption) {
    if (!confirm(`Delete the saved voice "${v.name}"? This cannot be undone.`)) return;

    try {
      await deleteCloneVoice(v.id);
      // Clear the selection before reloading, otherwise the panel keeps synthesising
      // against an id the backend no longer knows.
      if (selectedVoice === v.id) selectedVoice = '';
      toast.show(`Deleted voice "${v.name}".`, 'success');
      await loadVoicesForMode(activeMode.id);
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.show('Error deleting voice: ' + errMsg, 'error');
    }
  }

  function handleTrimmedAudio(_blob: Blob, trimmedFile: File) {
    isCloningTemp = true;
    toast.show('Uploading trimmed audio sample...', 'info');
    sendReference(trimmedFile)
      .then((path) => {
        referenceAudioPath = path;
        toast.show('Reference audio updated from trimmer!', 'success');
      })
      .catch((err) => {
        toast.show('Error uploading trimmed audio: ' + err.message, 'error');
      })
      .finally(() => {
        isCloningTemp = false;
      });
  }


  async function handleSynthesize() {
    if (!text.trim()) {
      toast.show('Please enter text content!', 'error');
      return;
    }

    if (supportsCloning && !referenceAudioPath && !selectedVoice) {
      toast.show('Please upload a reference audio file or select a cloned voice!', 'error');
      return;
    }

    // Every job goes through the streaming panel, including short text that fits in one
    // request — it simply becomes a one-chunk job. One code path means one place for
    // progress, cancellation, history and playback to be correct.
    const maxLimit = resolveStreamingThreshold(manifest);
    if (text.length > maxLimit) {
      toast.show(`Text length (${text.length} chars) exceeds single request limit (${maxLimit} chars). Automatically processing via Chunk Streaming!`, 'info');
    }
    requestStreaming();
  }
</script>

<div class="tab-content active" id="{activeMode.id}-tab">
  <!-- Dynamic Notice Banner from Manifest -->
  {#if modelOption?.notice_banner}
    {@const banner = modelOption.notice_banner}
    {@const styleMap = {
      info: { bg: 'rgba(59, 130, 246, 0.12)', border: 'rgba(59, 130, 246, 0.3)', color: '#60a5fa', icon: 'fa-circle-info' },
      success: { bg: 'rgba(34, 197, 94, 0.12)', border: 'rgba(34, 197, 94, 0.3)', color: '#4ade80', icon: 'fa-circle-check' },
      danger: { bg: 'rgba(239, 68, 68, 0.12)', border: 'rgba(239, 68, 68, 0.3)', color: '#f87171', icon: 'fa-circle-xmark' },
      error: { bg: 'rgba(239, 68, 68, 0.12)', border: 'rgba(239, 68, 68, 0.3)', color: '#f87171', icon: 'fa-circle-xmark' },
      warning: { bg: 'rgba(245, 158, 11, 0.12)', border: 'rgba(245, 158, 11, 0.3)', color: '#fbbf24', icon: 'fa-triangle-exclamation' }
    }}
    {@const currentStyle = styleMap[banner.level as keyof typeof styleMap] || styleMap.warning}
    <div style="background: {currentStyle.bg}; border: 1px solid {currentStyle.border}; border-radius: 8px; padding: 10px 14px; margin-bottom: 15px; display: flex; align-items: center; gap: 10px; color: {currentStyle.color}; font-size: 0.88em;">
      <i class="fa-solid {currentStyle.icon}" style="font-size: 1.1em; flex-shrink: 0;"></i>
      <span>{banner.message}</span>
    </div>
  {/if}

  <!-- Preset Voice Selection Widget -->
  {#if supportsPresetVoices || supportsVoiceSaving}
    <div class="form-group" style="margin-bottom: 1.2rem;">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
        <label for="generic-voice-select" style="margin-bottom: 0;">Voice Speaker</label>
        {#if supportsVoiceSaving}
          <button
            onclick={() => isCreateModalOpen = true}
            type="button"
            style="flex: 0 0 auto; padding: 6px 14px; border-radius: 8px; display: flex; align-items: center; gap: 6px; font-size: 0.85rem; font-weight: 600; white-space: nowrap; background: linear-gradient(135deg, #a855f7, #6366f1); color: white; border: none; cursor: pointer; box-shadow: 0 4px 12px rgba(168,85,247,0.3);"
            title="Create and save new voice to library"
          >
            <i class="fa-solid fa-plus"></i> <span>Save New Voice</span>
          </button>
        {/if}
      </div>

      {#if activeVoices.length === 0}
        <div style="padding: 12px 16px; background: rgba(255,255,255,0.04); border: 1px dashed rgba(255,255,255,0.15); border-radius: 10px; color: var(--text-muted); font-size: 0.9rem; display: flex; justify-content: space-between; align-items: center; gap: 10px; flex-wrap: wrap;">
          <span>No voice profiles saved yet.</span>
          {#if supportsVoiceSaving}
            <button
              onclick={() => isCreateModalOpen = true}
              type="button"
              style="background: none; border: none; color: var(--primary); font-weight: 600; cursor: pointer; text-decoration: underline; font-size: 0.9rem;"
            >
              + Create your first voice
            </button>
          {/if}
        </div>
      {:else if modelOption?.voice_type === 'radio'}
        <div style="display: flex; gap: 15px; margin-top: 8px; flex-wrap: wrap;">
          {#each activeVoices as v (v.id)}
            <label style="display: flex; align-items: center; gap: 8px; cursor: pointer; background: {selectedVoice === v.id || selectedVoice === v.name ? 'rgba(99,102,241,0.25)' : 'rgba(255,255,255,0.06)'}; padding: 10px 18px; border-radius: 8px; border: 1px solid {selectedVoice === v.id || selectedVoice === v.name ? 'var(--primary)' : 'rgba(255,255,255,0.15)'}; font-weight: 500; transition: all 0.2s;">
              <input
                type="radio"
                name="generic-voice-radio"
                value={v.id || v.name}
                bind:group={selectedVoice}
                style="accent-color: var(--primary); transform: scale(1.2);"
              />
              {#if v.metadata?.gender === 'female'}
                <i class="fa-solid fa-venus" style="color: #ff75a0;"></i>
              {:else if v.metadata?.gender === 'male'}
                <i class="fa-solid fa-mars" style="color: #4da6ff;"></i>
              {/if}
              <span>{v.name}</span>
            </label>
          {/each}
        </div>
      {:else}
        <VoiceSelect
          voices={activeVoices}
          selectedVoiceId={selectedVoice}
          onSelect={(v) => { selectedVoice = v.id || v.name; referenceAudioPath = ''; }}
          onDelete={handleDeleteVoice}
        />
      {/if}
    </div>
  {/if}

  <!-- Voice Cloning / Reference Audio Section -->
  {#if supportsCloning}
    <div class="clone-setup" style="margin-bottom: 1.2rem;">
      <label for="temp-voice-dropzone" style="font-weight: 600; color: #cbd5e1; display: block; margin-bottom: 6px; font-size: 0.95rem;">
        Reference Audio (Sample voice clip for cloning):
      </label>
      <label
        for="temp-voice-dropzone"
        class="dropzone-area"
        style="border: 2px dashed rgba(255,255,255,0.2); border-radius: 12px; padding: 20px; text-align: center; background: rgba(0,0,0,0.2); cursor: pointer; transition: all 0.2s; display: block;"
      >
        {#if referenceAudioPath}
          <div style="color: var(--success); font-weight: 500; display: flex; align-items: center; justify-content: center; gap: 8px;">
            <i class="fa-solid fa-file-audio" style="font-size: 1.2rem;"></i> Reference audio file active
          </div>
        {:else}
          <div style="color: var(--text-muted); display: flex; flex-direction: column; align-items: center; gap: 6px;">
            <i class="fa-solid fa-cloud-arrow-up" style="font-size: 1.8rem; color: var(--primary);"></i>
            <span>Drag & drop audio file here or <strong style="color: var(--primary);">click to browse</strong></span>
            <span style="font-size: 0.8rem; opacity: 0.7;">Supported formats: {supportedFormatsLabel(manifest, activeMode)}</span>
          </div>
        {/if}
        <input id="temp-voice-dropzone" aria-label="Upload reference audio file" type="file" onchange={handleFileUpload} accept={acceptedUploadFormats(manifest, activeMode)} style="display: none;" />
      </label>

      {#if selectedFile}
        <div style="margin-top: 10px;">
          <WaveformTrimmer file={selectedFile} onTrimmed={handleTrimmedAudio} {manifest} mode={activeMode} />
        </div>
      {/if}
    </div>
  {/if}

  <!-- Speed Control Widget -->
  {#if supportsSpeed}
    <div class="form-group">
      <label for="generic-speed">Speed: <span>{speed.toFixed(1)}x</span></label>
      {#if modelOption?.speed_type === 'number'}
        <input
          type="number"
          id="generic-speed"
          min={speedBounds.min}
          max={speedBounds.max}
          step={speedBounds.step}
          bind:value={speed}
          style="width: 100%; padding: 8px 12px; border-radius: 8px; background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.15); color: white;"
        />
      {:else}
        <input
          type="range"
          id="generic-speed"
          min={speedBounds.min}
          max={speedBounds.max}
          step={speedBounds.step}
          bind:value={speed}
        />
      {/if}
    </div>
  {/if}

  <!-- Pitch Control Widget (Dynamic) -->
  {#if supportsPitch}
    <div class="form-group">
      <label for="generic-pitch">Pitch: <span>{pitch.toFixed(1)}</span></label>
      {#if modelOption?.pitch_type === 'number'}
        <input
          type="number"
          id="generic-pitch"
          min={pitchBounds.min}
          max={pitchBounds.max}
          step={pitchBounds.step}
          bind:value={pitch}
          style="width: 100%; padding: 8px 12px; border-radius: 8px; background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.15); color: white;"
        />
      {:else}
        <input
          type="range"
          id="generic-pitch"
          min={pitchBounds.min}
          max={pitchBounds.max}
          step={pitchBounds.step}
          bind:value={pitch}
        />
      {/if}
    </div>
  {/if}

  <!-- Emotion Control Widget (Dynamic) -->
  {#if supportsEmotion}
    <div class="form-group">
      <label for="generic-emotion">Emotion</label>
      {#if modelOption?.emotion_type === 'radio'}
        <div style="display: flex; gap: 12px; margin-top: 8px; flex-wrap: wrap;">
          {#each availableEmotions as em (em)}
            <label style="display: flex; align-items: center; gap: 6px; cursor: pointer; background: {selectedEmotion === em ? 'rgba(99,102,241,0.25)' : 'rgba(255,255,255,0.06)'}; padding: 8px 14px; border-radius: 8px; border: 1px solid {selectedEmotion === em ? 'var(--primary)' : 'rgba(255,255,255,0.15)'}; font-weight: 500;">
              <input type="radio" name="generic-emotion-radio" value={em} bind:group={selectedEmotion} style="accent-color: var(--primary);" />
              <span>{em}</span>
            </label>
          {/each}
        </div>
      {:else}
        <select
          id="generic-emotion"
          bind:value={selectedEmotion}
          style="width: 100%; padding: 8px 12px; border-radius: 8px; background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.15); color: white;"
        >
          <option value="">Default (Natural)</option>
          {#each availableEmotions as em (em)}
            <option value={em}>{em}</option>
          {/each}
        </select>
      {/if}
    </div>
  {/if}

  <!-- Synthesize Actions -->
  <div class="action-buttons">
    <button onclick={handleSynthesize} class="btn primary-btn">
      <i class="fa-solid {synthLabel.icon}"></i> {synthLabel.text}
    </button>
  </div>
</div>

<!-- Create New Clone Voice Modal -->
{#if isCreateModalOpen}
  <CreateVoiceModal
    isOpen={isCreateModalOpen}
    modelId={activeMode.id}
    metadataSchema={modelOption?.voice_metadata_schema}
    {manifest}
    mode={activeMode}
    onClose={() => isCreateModalOpen = false}
    onSaved={(voiceId, voiceName) => {
      isCreateModalOpen = false;
      loadVoicesForMode(activeMode.id).then(() => {
        selectedVoice = voiceId || voiceName;
      });
    }}
  />
{/if}
