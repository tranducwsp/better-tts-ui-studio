<script lang="ts">
  import type { VoiceOption } from '../types';
  import VoiceSelect from './VoiceSelect.svelte';
  import StreamingPanel from './StreamingPanel.svelte';
  import { synthesizeStandard, subscribeTaskStream, fetchVoices } from '../api';
  import { toast } from '../toast.svelte';

  interface Props {
    text: string;
    voices: VoiceOption[];
    reloadedJob?: any | null;
  }

  let { text, voices = $bindable([]), reloadedJob = null }: Props = $props();

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
  <div class="form-group">
    <label for="std-voice-select-trigger">Giọng đọc</label>
    <VoiceSelect
      {voices}
      bind:selectedVoiceId
      onSelect={(v) => selectedVoiceId = v.id}
    />
  </div>

  <div class="form-group">
    <label for="std-speed">Tốc độ: <span>{speed.toFixed(1)}x</span></label>
    <input type="range" id="std-speed" min="0.5" max="2.0" step="0.1" bind:value={speed} />
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
