<script lang="ts">
  import type { VoiceOption, ModelOptionSpec } from '../types';
  import VoiceSelect from './VoiceSelect.svelte';
  import StreamingPanel from './StreamingPanel.svelte';
  import { synthesizeStandard, subscribeTaskStream, fetchVoices } from '../api';
  import { toast } from '../toast.svelte';

  interface Props {
    text: string;
    voices: VoiceOption[];
    reloadedJob?: any | null;
    modelOption?: ModelOptionSpec | null;
  }

  let { text, voices = $bindable([]), reloadedJob = null, modelOption = null }: Props = $props();

  let selectedVoiceId = $state('');
  let speed = $state(1.0);
  let isLoading = $state(false);
  let isStreaming = $state(false);
  let progress = $state(0);
  let wavBlobUrl = $state<string | null>(null);
  let mp3AudioUrl = $state<string | null>(null);
  let unsubscribeStream = $state<(() => void) | null>(null);

  $effect(() => {
    if (!voices || voices.length === 0) {
      fetchVoices().then((v) => {
        if (v && v.length > 0) {
          voices = v;
        }
      });
    }
  });

  $effect(() => {
    if (voices && voices.length > 0 && !selectedVoiceId) {
      selectedVoiceId = voices[0].id;
    }
  });

  $effect(() => {
    if (reloadedJob && reloadedJob.engine === 'standard') {
      if (reloadedJob.voice) selectedVoiceId = reloadedJob.voice;
      if (reloadedJob.speed) speed = reloadedJob.speed;
      if (reloadedJob.chunks && reloadedJob.chunks.length > 0) {
        isStreaming = true;
      }
    }
  });

  function handleSynthesize() {
    if (!text.trim()) {
      toast.show('Vui lòng nhập nội dung văn bản trước khi tổng hợp!', 'error');
      return;
    }

    startSynthesis();
  }

  async function startSynthesis() {
    if (text.length > 1000) {
      isStreaming = true;
      return;
    }

    isStreaming = false;
    isLoading = true;
    progress = 0;
    wavBlobUrl = null;
    mp3AudioUrl = null;
    toast.show('Đang khởi tạo tiến trình...', 'info');

    try {
      const jobId = 'job_' + Date.now();
      const taskId = await synthesizeStandard(text, selectedVoiceId, speed, jobId, 0, 1);

      unsubscribeStream = subscribeTaskStream(
        taskId,
        (p) => {
          progress = p;
        },
        (blob, mp3Url) => {
          wavBlobUrl = URL.createObjectURL(blob);
          mp3AudioUrl = mp3Url;
          isLoading = false;
          toast.show('Tổng hợp âm thanh hoàn tất!', 'success');
        },
        (errMsg) => {
          isLoading = false;
          toast.show('Lỗi: ' + errMsg, 'error');
        }
      );
    } catch (err: any) {
      isLoading = false;
      toast.show('Lỗi: ' + err.message, 'error');
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

<div class="tab-content active">
  <!-- Cảnh báo Bảo mật từ Manifest -->
  {#if modelOption?.notice_banner}
    <div style="background: {modelOption.notice_banner.level === 'danger' ? 'rgba(239, 68, 68, 0.12)' : 'rgba(255, 193, 7, 0.12)'}; border: 1px solid {modelOption.notice_banner.level === 'danger' ? 'rgba(239, 68, 68, 0.3)' : 'rgba(255, 193, 7, 0.3)'}; border-radius: 8px; padding: 10px 14px; margin-bottom: 15px; display: flex; align-items: center; gap: 10px; color: {modelOption.notice_banner.level === 'danger' ? '#ef4444' : '#ffc107'}; font-size: 0.88em;">
      <i class="fa-solid fa-triangle-exclamation" style="font-size: 1.1em; flex-shrink: 0;"></i>
      <span>{modelOption.notice_banner.message}</span>
    </div>
  {/if}

  <div class="form-group">
    <label for="std-voice-select-trigger">Giọng đọc</label>
    {#if modelOption?.voice_type === 'radio'}
      <div style="display: flex; gap: 15px; margin-top: 8px; flex-wrap: wrap;">
        {#each voices as v (v.id)}
          <label style="display: flex; align-items: center; gap: 8px; cursor: pointer; background: {selectedVoiceId === v.id ? 'rgba(99,102,241,0.25)' : 'rgba(255,255,255,0.06)'}; padding: 10px 18px; border-radius: 8px; border: 1px solid {selectedVoiceId === v.id ? 'var(--primary)' : 'rgba(255,255,255,0.15)'}; font-weight: 500; transition: all 0.2s;">
            <input
              type="radio"
              name="std-voice-radio"
              value={v.id}
              bind:group={selectedVoiceId}
              style="accent-color: var(--primary); transform: scale(1.2);"
            />
            <span>{v.name}</span>
          </label>
        {/each}
      </div>
    {:else}
      <VoiceSelect
        {voices}
        bind:selectedVoiceId
        onSelect={(v) => selectedVoiceId = v.id}
      />
    {/if}
  </div>

  <div class="form-group">
    <label for="std-speed">Tốc độ: <span>{speed.toFixed(1)}x</span></label>
    {#if modelOption?.speed_type === 'number'}
      <input type="number" id="std-speed" min="0.5" max="2.0" step="0.1" bind:value={speed} style="width: 100%; padding: 8px 12px; border-radius: 8px; background: rgba(255,255,255,0.05); border: 1px solid rgba(255,255,255,0.15); color: white;" />
    {:else}
      <input type="range" id="std-speed" min="0.5" max="2.0" step="0.1" bind:value={speed} />
    {/if}
  </div>

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

  {#if isLoading}
    <div class="loading-indicator">
      <div class="spinner"></div>
      <p>Đang xử lý âm thanh AI ({progress}%)...</p>
      <div class="progress-wrapper">
        <div class="progress-bar" style="width: {progress}%"></div>
      </div>
    </div>
  {/if}

  {#if isStreaming}
    <StreamingPanel
      {text}
      engine="standard"
      voice={selectedVoiceId}
      {speed}
      {reloadedJob}
      onClose={() => isStreaming = false}
    />
  {/if}

  {#if wavBlobUrl && !isStreaming}
    <div class="audio-result" style="text-align: center;">
      <p class="status-msg success"><i class="fa-solid fa-circle-check"></i> Hoàn tất!</p>
      <audio controls src={wavBlobUrl} style="width: 100%; margin-bottom: 10px;"></audio>
      <div style="display: flex; gap: 10px; justify-content: center; flex-wrap: wrap;">
        <a href={wavBlobUrl} download="vieneu_tts.wav" class="btn secondary-btn" style="text-decoration: none;">
          <i class="fa-solid fa-download"></i> Tải .WAV (gốc)
        </a>
        {#if mp3AudioUrl}
          <a href={mp3AudioUrl} download="vieneu_tts.mp3" class="btn secondary-btn" style="text-decoration: none;">
            <i class="fa-solid fa-download"></i> Tải .MP3 (nhẹ)
          </a>
        {/if}
      </div>
    </div>
  {/if}
</div>
