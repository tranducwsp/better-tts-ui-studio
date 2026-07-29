<script lang="ts">
  import VoiceSelect from './VoiceSelect.svelte';
  import WaveformTrimmer from './WaveformTrimmer.svelte';
  import CreateVoiceModal from './CreateVoiceModal.svelte';
  import StreamingPanel from './StreamingPanel.svelte';
  import { fetchPresets, synthesizeClone, cloneVoiceTemp, deleteCloneVoice, subscribeTaskStream } from '../api';
  import { toast } from '../toast.svelte';
  import type { Preset, VoiceOption } from '../types';

  interface Props {
    text: string;
    reloadedJob?: any | null;
  }

  let { text, reloadedJob = null }: Props = $props();

  // State
  let presets = $state<Preset[]>([]);
  let selectedPresetId = $state<string>('');
  let isCreateModalOpen = $state(false);

  // Convert presets to VoiceOption format for VoiceSelect component
  let cloneVoiceOptions = $derived.by<VoiceOption[]>(() => {
    return presets.map((p) => ({
      id: p.id,
      name: p.name || p.speaker,
      descriptions: (p as any).descriptions || [(p as any).gender, (p as any).region, (p as any).style].filter(Boolean)
    }));
  });

  // Temporary file state
  let tempFileInput = $state<HTMLInputElement | null>(null);
  let tempOriginalFile = $state<File | null>(null);
  let tempTrimmedFile = $state<File | null>(null);
  let activeCloneId = $state<string | null>(null);

  let speed = $state(1.0);
  let isCloningTemp = $state(false);
  let isSynthesizing = $state(false);
  let isStreaming = $state(false);
  let progress = $state(0);

  let wavBlobUrl = $state<string | null>(null);
  let mp3AudioUrl = $state<string | null>(null);
  let unsubscribeStream = $state<(() => void) | null>(null);

  $effect(() => {
    loadPresets();
  });

  $effect(() => {
    if (reloadedJob && reloadedJob.engine === 'clone') {
      if (reloadedJob.voice) {
        selectedPresetId = reloadedJob.voice;
        activeCloneId = reloadedJob.voice;
      }
      if (reloadedJob.speed) speed = reloadedJob.speed;
      if (reloadedJob.chunks && reloadedJob.chunks.length > 0) {
        isStreaming = true;
      }
    }
  });

  async function loadPresets() {
    try {
      const data = await fetchPresets();
      presets = data || [];
    } catch (e) {
      presets = [];
    }
  }

  function handleSelectPreset(presetId: string) {
    selectedPresetId = presetId;
    if (presetId) {
      activeCloneId = presetId;
      tempOriginalFile = null;
      tempTrimmedFile = null;
    }
  }

  async function handleDeleteVoice(v: VoiceOption) {
    if (confirm(`Bạn có chắc muốn xóa giọng mẫu "${v.name}" không?`)) {
      try {
        await deleteCloneVoice(v.id);
        toast.show('Đã xóa giọng mẫu thành công!', 'success');
        if (selectedPresetId === v.id) {
          selectedPresetId = '';
          activeCloneId = null;
        }
        await loadPresets();
      } catch (err: any) {
        toast.show('Lỗi xóa giọng mẫu: ' + err.message, 'error');
      }
    }
  }

  function handleTempFileSelect(e: Event) {
    const target = e.target as HTMLInputElement;
    if (target.files && target.files.length > 0) {
      const file = target.files[0];
      if (!file.name.toLowerCase().endsWith('.wav')) {
        toast.show('Chỉ chấp nhận file âm thanh định dạng .wav!', 'error');
        return;
      }
      tempOriginalFile = file;
      tempTrimmedFile = file;
      selectedPresetId = '';
      activeCloneId = null;
    }
  }

  async function handleTempCloneUpload() {
    const fileToUpload = tempTrimmedFile || tempOriginalFile;
    if (!fileToUpload) {
      toast.show('Vui lòng chọn file mẫu .wav!', 'error');
      return;
    }
    isCloningTemp = true;
    toast.show('Đang trích xuất đặc trưng giọng tạm...', 'info');

    try {
      const id = await cloneVoiceTemp(fileToUpload);
      activeCloneId = id;
      toast.show('Đã kích hoạt giọng mẫu tạm!', 'success');
    } catch (err: any) {
      toast.show('Lỗi trích xuất giọng: ' + err.message, 'error');
    } finally {
      isCloningTemp = false;
    }
  }

  function handleSynthesizeClone() {
    if (!text.trim()) {
      toast.show('Vui lòng nhập văn bản cần tổng hợp!', 'error');
      return;
    }
    if (!activeCloneId) {
      toast.show('Vui lòng chọn giọng mẫu đã lưu hoặc tải mẫu dùng tạm!', 'error');
      return;
    }

    startCloneSynthesis();
  }

  async function startCloneSynthesis() {
    if (text.length > 1000) {
      isStreaming = true;
      return;
    }

    isStreaming = false;
    isSynthesizing = true;
    progress = 0;
    wavBlobUrl = null;
    mp3AudioUrl = null;
    toast.show('Đang tổng hợp giọng clone...', 'info');

    try {
      const jobId = 'job_' + Date.now();
      const taskId = await synthesizeClone(text, activeCloneId!, speed, jobId, 0, 1);

      unsubscribeStream = subscribeTaskStream(
        taskId,
        (p) => {
          progress = p;
        },
        (blob, mp3Url) => {
          wavBlobUrl = URL.createObjectURL(blob);
          mp3AudioUrl = mp3Url;
          isSynthesizing = false;
          toast.show('Tổng hợp giọng clone hoàn tất!', 'success');
        },
        (errMsg) => {
          isSynthesizing = false;
          toast.show('Lỗi: ' + errMsg, 'error');
        }
      );
    } catch (err: any) {
      isSynthesizing = false;
      toast.show('Lỗi tổng hợp giọng clone: ' + err.message, 'error');
    }
  }

  function handleCancel() {
    if (unsubscribeStream) {
      unsubscribeStream();
      unsubscribeStream = null;
    }
    isSynthesizing = false;
    toast.show('Đã hủy tiến trình', 'info');
  }

  function handleVoiceSaved(newVoiceId: string, newVoiceName: string) {
    activeCloneId = newVoiceId;
    loadPresets();
    selectedPresetId = newVoiceId;
  }
</script>

<div class="tab-content active">
  <!-- Section 1: Saved Clone Voices Library -->
  <div class="form-group" style="margin-bottom: 1.2rem;">
    <label for="clone-voice-select-wrapper" style="font-weight: 600; color: #c084fc; display: flex; align-items: center; gap: 8px; margin-bottom: 8px;">
      <i class="fa-solid fa-folder-open"></i> Giọng mẫu đã lưu sẵn:
    </label>
    <div id="clone-voice-select-wrapper" style="display: flex; gap: 10px; align-items: center;">
      <div style="flex: 1;">
        <VoiceSelect
          voices={cloneVoiceOptions}
          bind:selectedVoiceId={selectedPresetId}
          placeholder="-- Hoặc chọn mẫu âm thanh dùng tạm bên dưới --"
          onSelect={(v) => handleSelectPreset(v.id)}
          onDelete={handleDeleteVoice}
        />
      </div>

      <button
        onclick={() => isCreateModalOpen = true}
        type="button"
        style="flex: 0 0 auto; padding: 0 18px; height: 48px; border-radius: 12px; display: flex; align-items: center; gap: 8px; font-weight: 600; white-space: nowrap; background: linear-gradient(135deg, #a855f7, #6366f1); color: white; border: none; cursor: pointer;"
        title="Tạo giọng mới lưu vào thư viện"
      >
        <i class="fa-solid fa-plus"></i> <span>Lưu giọng mới</span>
      </button>
    </div>
  </div>

  <!-- Section 2: Temporary 1-Time Voice Clone -->
  <div class="clone-setup" style="margin-top: 1.5rem;">
    <label for="temp-voice-dropzone" style="font-weight: 600; color: #94a3b8; display: block; margin-bottom: 6px; font-size: 0.95rem;">
      <i class="fa-solid fa-bolt" style="color: #fbbf24;"></i> Hoặc tải mẫu âm thanh dùng tạm 1 lần (.wav 3-5s):
    </label>

    <div id="temp-voice-dropzone" role="button" tabindex="0" class="upload-drop-zone" onclick={() => tempFileInput?.click()} onkeydown={(e) => e.key === 'Enter' && tempFileInput?.click()}>
      <i class="fa-solid fa-cloud-arrow-up" style="font-size: 2.5rem; color: var(--primary); margin-bottom: 10px;"></i>
      {#if tempOriginalFile}
        <p style="color: var(--success); font-weight: 600;"><i class="fa-solid fa-file-audio"></i> {tempOriginalFile.name}</p>
      {:else}
        <p>Kéo thả file âm thanh .wav hoặc <span style="color: var(--primary);">chọn file</span></p>
      {/if}
      <input type="file" bind:this={tempFileInput} onchange={handleTempFileSelect} accept=".wav,audio/wav" class="hidden" />
    </div>

    {#if tempOriginalFile}
      <WaveformTrimmer file={tempOriginalFile} onTrimmed={(_, file) => tempTrimmedFile = file} />
      <div style="margin-top: 10px; display: flex; justify-content: flex-end;">
        <button onclick={handleTempCloneUpload} disabled={isCloningTemp} class="btn primary-btn" style="padding: 10px 20px; font-size: 0.9rem;">
          <i class="fa-solid fa-wand-magic-sparkles"></i> {isCloningTemp ? 'Đang trích xuất...' : 'Kích hoạt mẫu tạm'}
        </button>
      </div>
    {/if}
  </div>

  <!-- Active Voice Status Banner -->
  {#if activeCloneId}
    <div class="clone-status" style="margin-top: 1rem; margin-bottom: 1rem; padding: 12px; background: rgba(16, 185, 129, 0.1); border: 1px solid rgba(16, 185, 129, 0.3); border-radius: 8px; color: var(--success);">
      <p><i class="fa-solid fa-circle-check"></i> ID Giọng Đang Chọn: <code>{activeCloneId}</code></p>
    </div>
  {/if}

  <!-- Section 3: Speed Slider & Synthesize Action -->
  <div class="form-group" style="margin-top: 1rem;">
    <label for="clone-speed">Tốc độ: <span>{speed.toFixed(1)}x</span></label>
    <input type="range" id="clone-speed" min="0.5" max="2.0" step="0.1" bind:value={speed} />
  </div>

  <div class="action-buttons">
    {#if isSynthesizing}
      <button onclick={handleCancel} class="btn secondary-btn" style="flex: 1; border-color: var(--danger); color: var(--danger);">
        <i class="fa-solid fa-ban"></i> Hủy tiến trình
      </button>
    {:else}
      <button onclick={handleSynthesizeClone} disabled={!activeCloneId} class="btn primary-btn">
        <i class="fa-solid fa-play"></i> Tổng hợp âm thanh
      </button>
    {/if}
  </div>

  {#if isSynthesizing}
    <div class="loading-indicator">
      <div class="spinner"></div>
      <p>Đang tổng hợp giọng clone ({progress}%)...</p>
      <div class="progress-wrapper">
        <div class="progress-bar" style="width: {progress}%"></div>
      </div>
    </div>
  {/if}

  {#if isStreaming && activeCloneId}
    <StreamingPanel
      {text}
      engine="clone"
      voice={activeCloneId}
      {speed}
      {reloadedJob}
      onClose={() => isStreaming = false}
    />
  {/if}

  <!-- Section 4: Result Player & Download links -->
  {#if wavBlobUrl && !isStreaming}
    <div class="audio-result" style="text-align: center;">
      <p class="status-msg success"><i class="fa-solid fa-circle-check"></i> Hoàn tất!</p>
      <audio controls src={wavBlobUrl} style="width: 100%; margin-bottom: 10px;"></audio>
      <div style="display: flex; gap: 10px; justify-content: center; flex-wrap: wrap;">
        <a href={wavBlobUrl} download="vieneu_tts_clone.wav" class="btn secondary-btn" style="text-decoration: none;">
          <i class="fa-solid fa-download"></i> Tải .WAV (gốc)
        </a>
        {#if mp3AudioUrl}
          <a href={mp3AudioUrl} download="vieneu_tts_clone.mp3" class="btn secondary-btn" style="text-decoration: none;">
            <i class="fa-solid fa-download"></i> Tải .MP3 (nhẹ)
          </a>
        {/if}
      </div>
    </div>
  {/if}
</div>

<!-- Modal Create Voice -->
<CreateVoiceModal
  isOpen={isCreateModalOpen}
  onClose={() => isCreateModalOpen = false}
  onSaved={handleVoiceSaved}
/>
