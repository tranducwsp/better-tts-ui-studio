<script lang="ts">
  import type { VoiceOption, UniversalManifest, EngineModeSpec } from '../types';
  import VoiceSelect from './VoiceSelect.svelte';
  import StreamingPanel from './StreamingPanel.svelte';
  import WaveformTrimmer from './WaveformTrimmer.svelte';
  import CreateVoiceModal from './CreateVoiceModal.svelte';
  import { synthesize, subscribeTaskStream, fetchVoices, cloneVoiceTemp } from '../api';
  import { toast } from '../toast.svelte';

  interface Props {
    text: string;
    activeMode: EngineModeSpec;
    manifest: UniversalManifest;
    voices: VoiceOption[];
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

  // States
  let selectedVoice = $state('');
  let speed = $state(manifest?.constraints?.speed_range?.default || 1.0);
  let pitch = $state(manifest?.constraints?.pitch_range?.default || 0.0);
  let selectedEmotion = $state('');
  let referenceAudioPath = $state('');
  let isCloningTemp = $state(false);
  let isCreateModalOpen = $state(false);

  // Streaming / Loading States
  let isLoading = $state(false);
  let isStreaming = $state(false);
  let progress = $state(0);
  let wavBlobUrl = $state<string | null>(null);
  let mp3AudioUrl = $state<string | null>(null);
  let unsubscribeStream = $state<(() => void) | null>(null);

  // Dynamically load mode-specific voices when activeMode changes
  $effect(() => {
    const currentModeId = activeMode.id;
    if (supportsPresetVoices || supportsVoiceSaving) {
      fetchVoices(currentModeId).then((v) => {
        modeVoices = v || [];
        if (modeVoices.length > 0 && !selectedVoice) {
          selectedVoice = modeVoices[0].id || modeVoices[0].name;
        }
      });
    }
  });

  // Set default voice when activeVoices list changes
  $effect(() => {
    if (activeVoices && activeVoices.length > 0 && (!selectedVoice || !activeVoices.some(v => (v.id || v.name) === selectedVoice))) {
      selectedVoice = activeVoices[0].id || activeVoices[0].name;
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
    toast.show('Đang xử lý file âm thanh mẫu...', 'info');
    try {
      const tempPath = await cloneVoiceTemp(file);
      referenceAudioPath = tempPath;
      toast.show('Đã nạp file âm thanh mẫu thành công!', 'success');
    } catch (err: any) {
      toast.show('Lỗi nạp file mẫu: ' + err.message, 'error');
    } finally {
      isCloningTemp = false;
      target.value = '';
    }
  }

  function handleTrimmedAudio(blob: Blob) {
    const file = new File([blob], 'trimmed_reference.wav', { type: 'audio/wav' });
    isCloningTemp = true;
    toast.show('Đang tải lên đoạn âm thanh đã cắt...', 'info');
    cloneVoiceTemp(file)
      .then((path) => {
        referenceAudioPath = path;
        toast.show('Đã cập nhật file mẫu từ bộ cắt âm thanh!', 'success');
      })
      .catch((err) => {
        toast.show('Lỗi tải file đã cắt: ' + err.message, 'error');
      })
      .finally(() => {
        isCloningTemp = false;
      });
  }

  async function handleSynthesize() {
    if (!text.trim()) {
      toast.show('Vui lòng nhập nội dung văn bản!', 'error');
      return;
    }

    if (supportsCloning && !referenceAudioPath && !selectedVoice) {
      toast.show('Vui lòng tải lên file âm thanh mẫu hoặc chọn giọng clone!', 'error');
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
          (doneWavUrl) => {
            isLoading = false;
            wavBlobUrl = doneWavUrl;
            toast.show('Tổng hợp âm thanh hoàn tất!', 'success');
          },
          (err) => {
            isLoading = false;
            toast.show('Lỗi tiến trình: ' + err, 'error');
          }
        );
      } else {
        toast.show('Yêu cầu đã gửi thành công!', 'success');
        isLoading = false;
      }
    } catch (err: any) {
      isLoading = false;
      toast.show('Lỗi tổng hợp: ' + err.message, 'error');
    }
  }

  function handleCancel() {
    if (unsubscribeStream) {
      unsubscribeStream();
      unsubscribeStream = null;
    }
    isLoading = false;
    toast.show('Đã hủy tiến trình', 'info');
  }
</script>

<div class="tab-content active" id="{activeMode.id}-tab">
  <!-- Dynamic Notice Banner from Manifest -->
  {#if modelOption?.notice_banner}
    <div style="background: {modelOption.notice_banner.level === 'danger' ? 'rgba(239, 68, 68, 0.12)' : 'rgba(255, 193, 7, 0.12)'}; border: 1px solid {modelOption.notice_banner.level === 'danger' ? 'rgba(239, 68, 68, 0.3)' : 'rgba(255, 193, 7, 0.3)'}; border-radius: 8px; padding: 10px 14px; margin-bottom: 15px; display: flex; align-items: center; gap: 10px; color: {modelOption.notice_banner.level === 'danger' ? '#ef4444' : '#ffc107'}; font-size: 0.88em;">
      <i class="fa-solid fa-triangle-exclamation" style="font-size: 1.1em; flex-shrink: 0;"></i>
      <span>{modelOption.notice_banner.message}</span>
    </div>
  {/if}

  <!-- Preset Voice Selection Widget -->
  {#if supportsPresetVoices && activeVoices.length > 0}
    <div class="form-group" style="margin-bottom: 1.2rem;">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
        <label for="generic-voice-select" style="margin-bottom: 0;">Giọng đọc</label>
        {#if supportsVoiceSaving}
          <button
            onclick={() => isCreateModalOpen = true}
            type="button"
            style="flex: 0 0 auto; padding: 6px 14px; border-radius: 8px; display: flex; align-items: center; gap: 6px; font-size: 0.85rem; font-weight: 600; white-space: nowrap; background: linear-gradient(135deg, #a855f7, #6366f1); color: white; border: none; cursor: pointer;"
            title="Tạo giọng mới lưu vào thư viện"
          >
            <i class="fa-solid fa-plus"></i> <span>Lưu giọng mới</span>
          </button>
        {/if}
      </div>

      {#if modelOption?.voice_type === 'radio'}
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
              {#if v.gender === 'female'}
                <i class="fa-solid fa-venus" style="color: #ff75a0;"></i>
              {:else if v.gender === 'male'}
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
    <div class="clone-setup" style="margin-top: 1.2rem; margin-bottom: 1.2rem;">
      <label for="temp-voice-dropzone" style="font-weight: 600; color: #94a3b8; display: block; margin-bottom: 6px; font-size: 0.95rem;">
        <i class="fa-solid fa-bolt" style="color: #fbbf24;"></i> Hoặc tải mẫu âm thanh dùng tạm 1 lần (.wav):
      </label>

      <div
        id="temp-voice-dropzone"
        role="button"
        tabindex="0"
        class="upload-drop-zone"
        onclick={() => fileInput?.click()}
        onkeydown={(e) => e.key === 'Enter' && fileInput?.click()}
      >
        <i class="fa-solid fa-cloud-arrow-up" style="font-size: 2.5rem; color: var(--primary); margin-bottom: 10px;"></i>
        {#if referenceAudioPath}
          <p style="color: var(--success); font-weight: 600;"><i class="fa-solid fa-file-audio"></i> Đã kích hoạt file âm thanh mẫu</p>
        {:else}
          <p>Kéo thả file âm thanh .wav hoặc <span style="color: var(--primary);">chọn file</span></p>
        {/if}
        <input type="file" bind:this={fileInput} onchange={handleFileUpload} accept=".wav,audio/wav" class="hidden" />
      </div>

      {#if referenceAudioPath}
        <div style="margin-top: 10px;">
          <WaveformTrimmer audioUrl={referenceAudioPath} onTrimComplete={handleTrimmedAudio} />
        </div>
      {/if}
    </div>
  {/if}

  <!-- Speed Control Widget -->
  {#if manifest.capabilities.supports_speed}
    <div class="form-group">
      <label for="generic-speed">Tốc độ: <span>{speed.toFixed(1)}x</span></label>
      {#if modelOption?.speed_type === 'number'}
        <input
          type="number"
          id="generic-speed"
          min={manifest.constraints.speed_range?.min || 0.5}
          max={manifest.constraints.speed_range?.max || 2.0}
          step={manifest.constraints.speed_range?.step || 0.1}
          bind:value={speed}
          style="width: 100%; padding: 8px 12px; border-radius: 8px; background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.15); color: white;"
        />
      {:else}
        <input
          type="range"
          id="generic-speed"
          min={manifest.constraints.speed_range?.min || 0.5}
          max={manifest.constraints.speed_range?.max || 2.0}
          step={manifest.constraints.speed_range?.step || 0.1}
          bind:value={speed}
        />
      {/if}
    </div>
  {/if}

  <!-- Pitch Control Widget (Dynamic) -->
  {#if manifest.capabilities.supports_pitch}
    <div class="form-group">
      <label for="generic-pitch">Cao độ (Pitch): <span>{pitch.toFixed(1)}</span></label>
      <input
        type="range"
        id="generic-pitch"
        min={manifest.constraints.pitch_range?.min || -10}
        max={manifest.constraints.pitch_range?.max || 10}
        step={manifest.constraints.pitch_range?.step || 0.5}
        bind:value={pitch}
      />
    </div>
  {/if}

  <!-- Emotion Control Widget (Dynamic) -->
  {#if manifest.capabilities.supports_emotion && manifest.constraints.supported_emotions?.length > 0}
    <div class="form-group">
      <label for="generic-emotion">Cảm xúc (Emotion)</label>
      <select
        id="generic-emotion"
        bind:value={selectedEmotion}
        style="width: 100%; padding: 8px 12px; border-radius: 8px; background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.15); color: white;"
      >
        <option value="">Mặc định (Tự nhiên)</option>
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
        <i class="fa-solid fa-ban"></i> Hủy tiến trình
      </button>
    {:else}
      <button onclick={handleSynthesize} class="btn primary-btn">
        <i class="fa-solid fa-play"></i> Tổng hợp âm thanh
      </button>
    {/if}
  </div>

  <!-- Loading / Streaming Panel -->
  {#if isLoading}
    <div class="loading-indicator">
      <div class="spinner"></div>
      <p>Đang xử lý âm thanh AI ({progress}%)...</p>
      <div class="progress-wrapper">
        <div class="progress-bar" style="width: {progress}%;"></div>
      </div>
    </div>
  {/if}

  {#if isStreaming}
    <StreamingPanel {wavBlobUrl} {mp3AudioUrl} />
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
