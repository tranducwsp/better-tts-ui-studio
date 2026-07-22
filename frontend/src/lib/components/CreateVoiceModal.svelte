<script lang="ts">
  import WaveformTrimmer from './WaveformTrimmer.svelte';
  import { cloneVoice } from '../api';
  import { toast } from '../toast.svelte';

  interface Props {
    isOpen: boolean;
    onClose: () => void;
    onSaved: (newVoiceId: string, newVoiceName: string) => void;
  }

  let { isOpen, onClose, onSaved }: Props = $props();

  let fileInput = $state<HTMLInputElement | null>(null);
  let selectedFile = $state<File | null>(null);
  let trimmedFile = $state<File | null>(null);
  let name = $state('');
  let gender = $state('Nam');
  let region = $state('Miền Bắc');
  let style = $state('Truyền cảm');
  let isSaving = $state(false);

  function handleFileSelect(e: Event) {
    const target = e.target as HTMLInputElement;
    if (target.files && target.files.length > 0) {
      selectedFile = target.files[0];
      trimmedFile = selectedFile;
      if (!name) name = selectedFile.name.replace(/\.[^/.]+$/, '');
    }
  }

  async function handleSaveVoice() {
    const fileToUpload = trimmedFile || selectedFile;
    if (!fileToUpload) {
      toast.show('Vui lòng chọn file âm thanh mẫu!', 'error');
      return;
    }
    if (!name.trim()) {
      toast.show('Vui lòng nhập tên giọng mẫu!', 'error');
      return;
    }

    isSaving = true;
    toast.show('Đang tải lên & lưu đặc trưng giọng...', 'info');

    try {
      const voiceId = await cloneVoice(fileToUpload, name);
      toast.show('Lưu giọng mới thành công!', 'success');
      onSaved(voiceId, name);
      onClose();
    } catch (err: any) {
      toast.show('Lỗi lưu giọng mẫu: ' + err.message, 'error');
    } finally {
      isSaving = false;
    }
  }
</script>

{#if isOpen}
  <div style="position: fixed; inset: 0; background: rgba(0,0,0,0.7); backdrop-filter: blur(8px); z-index: 9999; display: flex; align-items: center; justify-content: center; padding: 20px;">
    <div class="glass-panel modal-animate" style="width: 100%; max-width: 650px; max-height: 90vh; overflow-y: auto;">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; border-bottom: 1px solid var(--glass-border); padding-bottom: 15px;">
        <h3 style="color: var(--primary); display: flex; align-items: center; gap: 10px; font-size: 1.2rem;">
          <i class="fa-solid fa-microphone-lines"></i> Tạo & Lưu Giọng Mẫu Mới (Clone)
        </h3>
        <button onclick={onClose} aria-label="Đóng modal" style="background: rgba(255,255,255,0.1); color: white; border: none; width: 32px; height: 32px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 1rem; cursor: pointer;">
          <i class="fa-solid fa-xmark"></i>
        </button>
      </div>

      <!-- Upload area -->
      <div class="upload-drop-zone" onclick={() => fileInput?.click()} style="margin-bottom: 1rem;">
        <i class="fa-solid fa-cloud-arrow-up" style="font-size: 2.5rem; color: var(--primary); margin-bottom: 10px;"></i>
        {#if selectedFile}
          <p style="color: var(--success); font-weight: 600;">{selectedFile.name}</p>
        {:else}
          <p>Kéo thả file âm thanh .wav hoặc <span style="color: var(--primary);">chọn file</span></p>
        {/if}
        <input type="file" bind:this={fileInput} onchange={handleFileSelect} accept=".wav,audio/wav" class="hidden" />
      </div>

      <!-- Trimmer if file loaded -->
      {#if selectedFile}
        <WaveformTrimmer file={selectedFile} onTrimmed={(_, file) => trimmedFile = file} />
      {/if}

      <!-- Metadata fields form -->
      <div style="margin-top: 1.2rem; padding: 1rem; background: rgba(255, 255, 255, 0.03); border-radius: 12px; border: 1px solid rgba(255, 255, 255, 0.08);">
        <h4 style="margin-bottom: 0.8rem; font-size: 0.9rem; color: var(--primary); display: flex; align-items: center; gap: 6px;">
          <i class="fa-solid fa-bookmark"></i> Thông tin nhãn mác giọng mẫu
        </h4>

        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-bottom: 10px;">
          <div>
            <label for="modal-voice-name-input" style="font-size: 0.85rem; color: #94a3b8; display: block; margin-bottom: 4px;">Tên giọng mẫu *</label>
            <input
              id="modal-voice-name-input"
              type="text"
              bind:value={name}
              placeholder="Ví dụ: Giọng MC Nam..."
              style="width: 100%; height: 38px; padding: 0 12px; border-radius: 6px; background: rgba(15,23,42,0.6); border: 1px solid rgba(255,255,255,0.1); color: white;"
            />
          </div>
          <div>
            <label for="modal-voice-gender-select" style="font-size: 0.85rem; color: #94a3b8; display: block; margin-bottom: 4px;">Giới tính</label>
            <select
              id="modal-voice-gender-select"
              bind:value={gender}
              style="width: 100%; height: 38px; padding: 0 12px; border-radius: 6px; background: rgba(15,23,42,0.6); border: 1px solid rgba(255,255,255,0.1); color: white;"
            >
              <option value="Nam">Nam</option>
              <option value="Nữ">Nữ</option>
              <option value="Khác">Khác</option>
            </select>
          </div>
        </div>

        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px;">
          <div>
            <label for="modal-voice-region-select" style="font-size: 0.85rem; color: #94a3b8; display: block; margin-bottom: 4px;">Vùng miền</label>
            <select
              id="modal-voice-region-select"
              bind:value={region}
              style="width: 100%; height: 38px; padding: 0 12px; border-radius: 6px; background: rgba(15,23,42,0.6); border: 1px solid rgba(255,255,255,0.1); color: white;"
            >
              <option value="Miền Bắc">Miền Bắc</option>
              <option value="Miền Nam">Miền Nam</option>
              <option value="Miền Trung">Miền Trung</option>
              <option value="Khác">Khác</option>
            </select>
          </div>
          <div>
            <label for="modal-voice-style-select" style="font-size: 0.85rem; color: #94a3b8; display: block; margin-bottom: 4px;">Phong cách</label>
            <select
              id="modal-voice-style-select"
              bind:value={style}
              style="width: 100%; height: 38px; padding: 0 12px; border-radius: 6px; background: rgba(15,23,42,0.6); border: 1px solid rgba(255,255,255,0.1); color: white;"
            >
              <option value="Truyền cảm">Truyền cảm</option>
              <option value="Tin tức / Thời sự">Tin tức / Thời sự</option>
              <option value="Đọc truyện / Đọc sách">Đọc truyện / Đọc sách</option>
              <option value="Diễn cảm / Kịch tính">Diễn cảm / Kịch tính</option>
              <option value="Tự nhiên / Trò chuyện">Tự nhiên / Trò chuyện</option>
              <option value="Quảng cáo / Review">Quảng cáo / Review</option>
              <option value="Khác">Khác</option>
            </select>
          </div>
        </div>
      </div>

      <div style="display: flex; gap: 12px; justify-content: flex-end; margin-top: 20px;">
        <button onclick={onClose} class="btn secondary-btn" type="button"><i class="fa-solid fa-xmark"></i> Hủy</button>
        <button onclick={handleSaveVoice} disabled={isSaving || !selectedFile} class="btn primary-btn">
          <i class="fa-solid fa-cloud-arrow-up"></i> {isSaving ? 'Đang lưu...' : 'Tải Lên & Lưu Giọng'}
        </button>
      </div>
    </div>
  </div>
{/if}
