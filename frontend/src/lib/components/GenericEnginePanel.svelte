<script lang="ts">
  import type { VoiceOption, UniversalManifest, EngineModeSpec, Preset } from '../types';
  import VoiceSelect from './VoiceSelect.svelte';
  import StreamingPanel from './StreamingPanel.svelte';
  import WaveformTrimmer from './WaveformTrimmer.svelte';
  import CreateVoiceModal from './CreateVoiceModal.svelte';
  import { synthesize, subscribeTaskStream, fetchVoices, fetchPresets, cloneVoiceTemp } from '../api';
  import { toast } from '../toast.svelte';

  interface Props {
    text: string;
    activeMode: EngineModeSpec;
    manifest: UniversalManifest | null;
    voices?: VoiceOption[];
    reloadedJob?: any | null;
  }

  let { text, activeMode, manifest, voices = $bindable([]), reloadedJob = null }: Props = $props();

  // Mode option spec derived from manifest ui_schema
  let modelOption = $derived(manifest?.ui_schema?.option_panel?.[activeMode.id] || null);

  // Per-mode capabilities derived from activeMode with manifest fallbacks
  let supportsCloning = $derived(activeMode?.supports_cloning ?? false);
  let supportsVoiceSaving = $derived(activeMode?.supports_voice_saving ?? false);
  let supportsPresetVoices = $derived(
    activeMode?.supports_preset_voices ?? manifest?.capabilities?.supports_preset_voices ?? true
  );
  let supportsStreaming = $derived(
    activeMode?.supports_streaming ?? manifest?.capabilities?.supports_streaming ?? true
  );

  let modeVoices = $state<VoiceOption[]>([]);

  // Active voice list: use modelOption.preset_voices if specified by manifest, else modeVoices
  let activeVoices = $derived.by(() => {
    if (modelOption?.preset_voices && modelOption.preset_voices.length > 0) {
      return modelOption.preset_voices;
    }
    return modeVoices;
  });

  // Default derived constraints
  let defaultSpeed = $derived(manifest?.constraints?.speed_range?.default ?? 1.0);
  let defaultPitch = $derived(manifest?.constraints?.pitch_range?.default ?? 0.0);

  // States
  let selectedVoice = $state('');
  let speed = $state(1.0);
  let pitch = $state(0.0);
  let selectedEmotion = $state('');
  let referenceAudioPath = $state('');

  let isCloningTemp = $state(false);
  let isCreateModalOpen = $state(false);
  let fileInput = $state<HTMLInputElement | null>(null);
  let selectedFile = $state<File | null>(null);

  // Streaming / Loading States
  let isLoading = $state(false);
  let isStreaming = $state(false);
  let progress = $state(0);
  let wavBlobUrl = $state<string | null>(null);
  let mp3AudioUrl = $state<string | null>(null);
  let unsubscribeStream = $state<(() => void) | null>(null);

  // Load engine voices & user custom saved clone voices from DB
  async function loadVoicesForMode(modeId: string) {
    try {
      let combined: VoiceOption[] = [];
      const currentOption = manifest?.ui_schema?.option_panel?.[modeId] || null;

      // 1. Static preset voices from manifest schema if defined
      if (currentOption?.preset_voices && currentOption.preset_voices.length > 0) {
        combined = [...currentOption.preset_voices];
      } else {
        // 2. Fetch engine preset voices from /api/voices/{modeId}
        const engineVoices = await fetchVoices(modeId);
        if (engineVoices && engineVoices.length > 0) {
          combined = [...engineVoices];
        }
      }

      // 3. Fetch custom saved voices if mode supports voice saving
      if (activeMode?.supports_voice_saving) {
        const userPresets = await fetchPresets(modeId);
        if (userPresets && userPresets.length > 0) {
          const userVoices: VoiceOption[] = userPresets.map((p) => ({
            id: p.id,
            name: p.name,
            descriptions: (p as any).descriptions || [p.gender, p.region, p.style].filter((d): d is string => typeof d === 'string' && d.trim() !== '')
          }));
          combined = [...userVoices, ...combined];
        }
      }

      modeVoices = combined;
      if (combined.length > 0 && (!selectedVoice || !combined.some(v => (v.id || v.name) === selectedVoice))) {
        selectedVoice = combined[0].id || combined[0].name;
      }
    } catch (err) {
      console.error('Error loading voices for mode:', err);
    }
  }

  let lastModeId = $state('');

  $effect(() => {
    if (activeMode && activeMode.id && activeMode.id !== lastModeId) {
      lastModeId = activeMode.id;
      speed = manifest?.constraints?.speed_range?.default ?? 1.0;
      pitch = manifest?.constraints?.pitch_range?.default ?? 0.0;
      selectedVoice = '';
      loadVoicesForMode(activeMode.id);
    }
  });

  // Handle reloaded job state
  $effect(() => {
    if (reloadedJob && reloadedJob.engine === activeMode.id) {
      if (reloadedJob.voice) selectedVoice = reloadedJob.voice;
      if (reloadedJob.speed) speed = reloadedJob.speed;
      if (reloadedJob.chunks && reloadedJob.chunks.length > 0) {
        isStreaming = true;
      }
    }
  });

  async function handleFileUpload(e: Event) {
    const target = e.target as HTMLInputElement;
    if (!target.files?.length) return;
    const file = target.files[0];
    isCloningTemp = true;
    toast.show('Processing reference audio file...', 'info');
    try {
      const tempPath = await cloneVoiceTemp(file);
      referenceAudioPath = tempPath;
      toast.show('Reference audio file loaded successfully!', 'success');
    } catch (err: any) {
      toast.show('Error loading reference file: ' + err.message, 'error');
    } finally {
      isCloningTemp = false;
      target.value = '';
    }
  }

  function handleTrimmedAudio(blob: Blob) {
    const file = new File([blob], 'trimmed_reference.wav', { type: 'audio/wav' });
    isCloningTemp = true;
    toast.show('Uploading trimmed audio sample...', 'info');
    cloneVoiceTemp(file)
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

    isLoading = true;
    isStreaming = false;
    progress = 0;
    wavBlobUrl = null;
    mp3AudioUrl = null;

    try {
      const voiceParam = referenceAudioPath || selectedVoice;
      const taskId = await synthesize(text, voiceParam, speed, activeMode.id);

      if (supportsStreaming) {
        isStreaming = true;
        unsubscribeStream = subscribeTaskStream(
          taskId,
          (prog) => {
            progress = prog;
          },
          (doneWavBlob: any, mp3Url?: string) => {
            isLoading = false;
            isStreaming = false;
            if (doneWavBlob instanceof Blob) {
              wavBlobUrl = URL.createObjectURL(doneWavBlob);
            } else if (typeof doneWavBlob === 'string') {
              wavBlobUrl = doneWavBlob;
            }
            if (mp3Url) {
              mp3AudioUrl = mp3Url;
            }
            toast.show('Audio synthesis completed!', 'success');
          },
          (err) => {
            isLoading = false;
            isStreaming = false;
            toast.show('Process error: ' + err, 'error');
          }
        );
      } else {
        toast.show('Request submitted successfully!', 'success');
        isLoading = false;
      }
    } catch (err: any) {
      isLoading = false;
      toast.show('Synthesis error: ' + err.message, 'error');
    }
  }

  function handleCancel() {
    if (unsubscribeStream) {
      unsubscribeStream();
      unsubscribeStream = null;
    }
    isLoading = false;
    toast.show('Process cancelled', 'info');
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
    {@const currentStyle = (styleMap as Record<string, any>)[banner.level] || styleMap.warning}
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
              {#if (v as any).gender === 'female'}
                <i class="fa-solid fa-venus" style="color: #ff75a0;"></i>
              {:else if (v as any).gender === 'male'}
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
          onSelect={(v) => selectedVoice = v.id || v.name}
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
      <div
        role="button"
        tabindex="0"
        class="dropzone-area"
        onclick={() => fileInput?.click()}
        onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && fileInput?.click()}
        style="border: 2px dashed rgba(255,255,255,0.2); border-radius: 12px; padding: 20px; text-align: center; background: rgba(0,0,0,0.2); cursor: pointer; transition: all 0.2s;"
      >
        {#if referenceAudioPath}
          <div style="color: var(--success); font-weight: 500; display: flex; align-items: center; justify-content: center; gap: 8px;">
            <i class="fa-solid fa-file-audio" style="font-size: 1.2rem;"></i> Reference audio file active
          </div>
        {:else}
          <div style="color: var(--text-muted); display: flex; flex-direction: column; align-items: center; gap: 6px;">
            <i class="fa-solid fa-cloud-arrow-up" style="font-size: 1.8rem; color: var(--primary);"></i>
            <span>Drag & drop audio file here or <strong style="color: var(--primary);">click to browse</strong></span>
            <span style="font-size: 0.8rem; opacity: 0.7;">Supported formats: WAV (Max 10MB)</span>
          </div>
        {/if}
        <input id="temp-voice-dropzone" aria-label="Upload reference audio file" type="file" bind:this={fileInput} onchange={handleFileUpload} accept=".wav,audio/wav" class="hidden" />
      </div>

      {#if selectedFile}
        <div style="margin-top: 10px;">
          <WaveformTrimmer file={selectedFile} onTrimmed={handleTrimmedAudio} />
        </div>
      {/if}
    </div>
  {/if}

  <!-- Speed Control Widget -->
  {#if manifest?.capabilities?.supports_speed ?? true}
    <div class="form-group">
      <label for="generic-speed">Speed: <span>{speed.toFixed(1)}x</span></label>
      {#if modelOption?.speed_type === 'number'}
        <input
          type="number"
          id="generic-speed"
          min={manifest?.constraints?.speed_range?.min || 0.5}
          max={manifest?.constraints?.speed_range?.max || 2.0}
          step={manifest?.constraints?.speed_range?.step || 0.1}
          bind:value={speed}
          style="width: 100%; padding: 8px 12px; border-radius: 8px; background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.15); color: white;"
        />
      {:else}
        <input
          type="range"
          id="generic-speed"
          min={manifest?.constraints?.speed_range?.min || 0.5}
          max={manifest?.constraints?.speed_range?.max || 2.0}
          step={manifest?.constraints?.speed_range?.step || 0.1}
          bind:value={speed}
        />
      {/if}
    </div>
  {/if}

  <!-- Pitch Control Widget (Dynamic) -->
  {#if manifest?.capabilities?.supports_pitch}
    <div class="form-group">
      <label for="generic-pitch">Pitch: <span>{pitch.toFixed(1)}</span></label>
      <input
        type="range"
        id="generic-pitch"
        min={manifest?.constraints?.pitch_range?.min || -10}
        max={manifest?.constraints?.pitch_range?.max || 10}
        step={manifest?.constraints?.pitch_range?.step || 0.5}
        bind:value={pitch}
      />
    </div>
  {/if}

  <!-- Emotion Control Widget (Dynamic) -->
  {#if manifest?.capabilities?.supports_emotion && (manifest?.constraints?.supported_emotions?.length || 0) > 0}
    <div class="form-group">
      <label for="generic-emotion">Emotion</label>
      <select
        id="generic-emotion"
        bind:value={selectedEmotion}
        style="width: 100%; padding: 8px 12px; border-radius: 8px; background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.15); color: white;"
      >
        <option value="">Default (Natural)</option>
        {#each manifest.constraints.supported_emotions as em}
          <option value={em}>{em}</option>
        {/each}
      </select>
    </div>
  {/if}

  <!-- Synthesize Actions -->
  <div class="action-buttons">
    {#if isLoading}
      <button onclick={handleCancel} class="btn secondary-btn" style="flex: 1; border-color: var(--danger); color: var(--danger);">
        <i class="fa-solid fa-ban"></i> Cancel Process
      </button>
    {:else}
      <button onclick={handleSynthesize} class="btn primary-btn">
        <i class="fa-solid fa-play"></i> Synthesize Audio
      </button>
    {/if}
  </div>

  <!-- Loading / Streaming Panel -->
  {#if isLoading}
    <div class="loading-indicator">
      <div class="spinner"></div>
      <p>Processing AI Audio ({progress}%)...</p>
      <div class="progress-wrapper">
        <div class="progress-bar" style="width: {progress}%;"></div>
      </div>
    </div>
  {/if}

  <!-- Audio Result Player Card -->
  {#if wavBlobUrl}
    <div class="audio-result-card" style="margin-top: 1.5rem; padding: 1.25rem; background: rgba(255, 255, 255, 0.05); border: 1px solid var(--glass-border); border-radius: 14px;">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.8rem;">
        <h4 style="margin: 0; display: flex; align-items: center; gap: 8px; color: var(--primary);">
          <i class="fa-solid fa-circle-play"></i> Synthesized Audio Ready
        </h4>
        <span style="font-size: 0.82em; opacity: 0.7;">WAV 24kHz</span>
      </div>
      <audio controls autoplay src={wavBlobUrl} style="width: 100%; margin-bottom: 1rem; border-radius: 8px; outline: none;"></audio>
      <div style="display: flex; gap: 10px; flex-wrap: wrap;">
        <a href={wavBlobUrl} download="synthesized_audio.wav" class="btn btn-primary" style="text-decoration: none; padding: 8px 16px; font-size: 0.9em; display: inline-flex; align-items: center; gap: 6px;">
          <i class="fa-solid fa-download"></i> Download WAV
        </a>
        {#if mp3AudioUrl}
          <a href={mp3AudioUrl} download="synthesized_audio.mp3" class="btn btn-secondary" style="text-decoration: none; padding: 8px 16px; font-size: 0.9em; display: inline-flex; align-items: center; gap: 6px;">
            <i class="fa-solid fa-file-audio"></i> Download MP3
          </a>
        {/if}
      </div>
    </div>
  {/if}
</div>

<!-- Modal Tạo Giọng Clone Mới -->
{#if isCreateModalOpen}
  <CreateVoiceModal
    isOpen={isCreateModalOpen}
    modelId={activeMode.id}
    metadataSchema={modelOption?.voice_metadata_schema}
    onClose={() => isCreateModalOpen = false}
    onSaved={(voiceId, voiceName) => {
      isCreateModalOpen = false;
      fetchVoices(activeMode.id).then((v) => {
        modeVoices = v || [];
        selectedVoice = voiceId || voiceName;
      });
    }}
  />
{/if}
