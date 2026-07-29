<script lang="ts">
  import type { VoiceOption, ModelOptionSpec } from '../types';
  import StreamingPanel from './StreamingPanel.svelte';
  import { synthesizeFast, subscribeTaskStream } from '../api';
  import { toast } from '../toast.svelte';

  interface Props {
    text: string;
    voices?: VoiceOption[];
    reloadedJob?: any | null;
    modelOption?: ModelOptionSpec | null;
  }

  let { text, reloadedJob = null, modelOption = null }: Props = $props();

  let selectedVoice = $state('Hoài Mỹ (Nữ)');
  let speed = $state(1.0);
  let isLoading = $state(false);
  let isStreaming = $state(false);
  let progress = $state(0);
  let wavBlobUrl = $state<string | null>(null);
  let mp3AudioUrl = $state<string | null>(null);
  let unsubscribeStream = $state<(() => void) | null>(null);

  $effect(() => {
    if (reloadedJob && reloadedJob.engine === 'fasttts') {
      if (reloadedJob.voice) selectedVoice = reloadedJob.voice;
      if (reloadedJob.speed) speed = reloadedJob.speed;
      if (reloadedJob.chunks && reloadedJob.chunks.length > 0) {
        isStreaming = true;
      }
    }
  });

  function handleFastSynthesize() {
    if (!text.trim()) {
      toast.show('Vui lòng nhập nội dung văn bản!', 'error');
      return;
    }

    startFastSynthesis();
  }

  async function startFastSynthesis() {
    if (text.length > 1000) {
      isStreaming = true;
      return;
    }

    isStreaming = false;
    isLoading = true;
    progress = 0;
    wavBlobUrl = null;
    mp3AudioUrl = null;
    toast.show('Đang tạo tiến trình Siêu Nhanh...', 'info');

    try {
      const jobId = 'job_' + Date.now();
      const taskId = await synthesizeFast(text, selectedVoice, speed, jobId, 0, 1);

      unsubscribeStream = subscribeTaskStream(
        taskId,
        (p) => {
          progress = p;
        },
        (blob, mp3Url) => {
          wavBlobUrl = URL.createObjectURL(blob);
          mp3AudioUrl = mp3Url;
          isLoading = false;
          toast.show('Tổng hợp siêu nhanh thành công!', 'success');
        },
        (errMsg) => {
          isLoading = false;
          toast.show('Lỗi: ' + errMsg, 'error');
        }
      );
    } catch (err: any) {
      isLoading = false;
      toast.show('Lỗi tổng hợp nhanh: ' + err.message, 'error');
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

<div class="tab-content active" id="fasttts-tab">
  <!-- Cảnh báo Bảo mật từ Manifest -->
  {#if modelOption?.notice_banner}
    <div style="background: {modelOption.notice_banner.level === 'danger' ? 'rgba(239, 68, 68, 0.12)' : 'rgba(255, 193, 7, 0.12)'}; border: 1px solid {modelOption.notice_banner.level === 'danger' ? 'rgba(239, 68, 68, 0.3)' : 'rgba(255, 193, 7, 0.3)'}; border-radius: 8px; padding: 10px 14px; margin-bottom: 15px; display: flex; align-items: center; gap: 10px; color: {modelOption.notice_banner.level === 'danger' ? '#ef4444' : '#ffc107'}; font-size: 0.88em;">
      <i class="fa-solid fa-triangle-exclamation" style="font-size: 1.1em; flex-shrink: 0;"></i>
      <span>{modelOption.notice_banner.message}</span>
    </div>
  {/if}

  <!-- Chọn Giọng Đọc (Radio Buttons) -->
  <div class="form-group">
    <label for="fasttts-voice-group">Chọn Giọng Đọc</label>
    <div id="fasttts-voice-group" style="display: flex; gap: 15px; margin-top: 8px; flex-wrap: wrap;">
      <label style="display: flex; align-items: center; gap: 8px; cursor: pointer; background: {selectedVoice === 'Hoài Mỹ (Nữ)' ? 'rgba(99,102,241,0.25)' : 'rgba(255,255,255,0.06)'}; padding: 10px 18px; border-radius: 8px; border: 1px solid {selectedVoice === 'Hoài Mỹ (Nữ)' ? 'var(--primary)' : 'rgba(255,255,255,0.15)'}; font-weight: 500; transition: all 0.2s;">
        <input
          type="radio"
          name="fasttts-voice-radio"
          value="Hoài Mỹ (Nữ)"
          bind:group={selectedVoice}
          style="accent-color: var(--primary); transform: scale(1.2);"
        />
        <i class="fa-solid fa-venus" style="color: #ff75a0;"></i> Hoài Mỹ (Nữ)
      </label>

      <label style="display: flex; align-items: center; gap: 8px; cursor: pointer; background: {selectedVoice === 'Nam Minh (Nam)' ? 'rgba(99,102,241,0.25)' : 'rgba(255,255,255,0.06)'}; padding: 10px 18px; border-radius: 8px; border: 1px solid {selectedVoice === 'Nam Minh (Nam)' ? 'var(--primary)' : 'rgba(255,255,255,0.15)'}; font-weight: 500; transition: all 0.2s;">
        <input
          type="radio"
          name="fasttts-voice-radio"
          value="Nam Minh (Nam)"
          bind:group={selectedVoice}
          style="accent-color: var(--primary); transform: scale(1.2);"
        />
        <i class="fa-solid fa-mars" style="color: #4da6ff;"></i> Nam Minh (Nam)
      </label>
    </div>
  </div>

  <div class="form-group">
    <label for="fasttts-speed">Tốc độ: <span>{speed.toFixed(1)}x</span></label>
    <input type="range" id="fasttts-speed" min="0.5" max="2.0" step="0.1" bind:value={speed} />
  </div>

  <div class="action-buttons">
    {#if isLoading}
      <button onclick={handleCancel} class="btn secondary-btn" style="flex: 1; border-color: var(--danger); color: var(--danger);">
        <i class="fa-solid fa-ban"></i> Hủy tiến trình
      </button>
    {:else}
      <button onclick={handleFastSynthesize} class="btn primary-btn">
        <i class="fa-solid fa-bolt"></i> Tổng hợp Siêu Nhanh
      </button>
    {/if}
  </div>

  {#if isLoading}
    <div class="loading-indicator">
      <div class="spinner"></div>
      <p>Đang ghép luồng âm thanh siêu nhanh ({progress}%)...</p>
      <div class="progress-wrapper">
        <div class="progress-bar" style="width: {progress}%"></div>
      </div>
    </div>
  {/if}

  {#if isStreaming}
    <StreamingPanel
      {text}
      engine="fasttts"
      voice={selectedVoice}
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
        <a href={wavBlobUrl} download="fast_tts.wav" class="btn secondary-btn" style="text-decoration: none;">
          <i class="fa-solid fa-download"></i> Tải .WAV (gốc)
        </a>
        {#if mp3AudioUrl}
          <a href={mp3AudioUrl} download="fast_tts.mp3" class="btn secondary-btn" style="text-decoration: none;">
            <i class="fa-solid fa-download"></i> Tải .MP3 (nhẹ)
          </a>
        {/if}
      </div>
    </div>
  {/if}
</div>
