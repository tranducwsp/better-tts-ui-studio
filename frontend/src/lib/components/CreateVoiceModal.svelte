<script lang="ts">
  import WaveformTrimmer from './WaveformTrimmer.svelte';
  import { cloneVoice } from '../api';
  import { toast } from '../toast.svelte';
  import type { VoiceMetadataFieldSpec } from '../types';

  interface Props {
    isOpen: boolean;
    onClose: () => void;
    onSaved: (newVoiceId: string, newVoiceName: string) => void;
    modelId?: string;
    metadataSchema?: VoiceMetadataFieldSpec[];
  }

  let { isOpen, onClose, onSaved, modelId = 'clone', metadataSchema }: Props = $props();

  let fileInput = $state<HTMLInputElement | null>(null);
  let selectedFile = $state<File | null>(null);
  let trimmedFile = $state<File | null>(null);
  let isSaving = $state(false);

  let formData = $state<Record<string, string>>({
    name: '',
    gender: 'Nam',
    region: 'Miền Bắc',
    style: 'Truyền cảm'
  });

  let fields = $derived.by(() => {
    if (metadataSchema && metadataSchema.length > 0) {
      return metadataSchema;
    }
    return [
      { key: 'name', label: 'Tên giọng mẫu', type: 'text', required: true, placeholder: 'Ví dụ: Giọng MC Nam...' },
      { key: 'gender', label: 'Giới tính', type: 'select', options: ['Nam', 'Nữ', 'Khác'] },
      { key: 'region', label: 'Vùng miền', type: 'select', options: ['Miền Bắc', 'Miền Nam', 'Miền Trung', 'Khác'] },
      { key: 'style', label: 'Phong cách', type: 'select', options: ['Truyền cảm', 'Tin tức / Thời sự', 'Đọc truyện / Đọc sách', 'Diễn cảm / Kịch tính', 'Tự nhiên / Trò chuyện', 'Quảng cáo / Review', 'Khác'] }
    ];
  });

  function handleFileSelect(e: Event) {
    const target = e.target as HTMLInputElement;
    if (target.files && target.files.length > 0) {
      selectedFile = target.files[0];
      trimmedFile = selectedFile;
      if (!formData['name']) {
        formData['name'] = selectedFile.name.replace(/\.[^/.]+$/, '');
      }
    }
  }

  async function handleSaveVoice() {
    const fileToUpload = trimmedFile || selectedFile;
    if (!fileToUpload) {
      toast.show('Vui lòng chọn file âm thanh mẫu!', 'error');
      return;
    }
    const nameVal = formData['name'] || '';
    if (!nameVal.trim()) {
      toast.show('Vui lòng nhập tên giọng mẫu!', 'error');
      return;
    }

    isSaving = true;
    toast.show('Đang tải lên & lưu đặc trưng giọng...', 'info');

    const genderVal = formData['gender'] || 'Nam';
    const regionVal = formData['region'] || 'Miền Bắc';
    const styleVal = formData['style'] || 'Truyền cảm';

    try {
      const voiceId = await cloneVoice(fileToUpload, nameVal, genderVal, regionVal, styleVal, modelId);
      toast.show('Lưu giọng mới thành công!', 'success');
      onSaved(voiceId, nameVal);
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
      <button type="button" class="upload-drop-zone" onclick={() => fileInput?.click()} style="width: 100%; margin-bottom: 1rem; background: none; border: 2px dashed rgba(255,255,255,0.15); border-radius: 12px; padding: 20px; text-align: center; cursor: pointer;">
        <i class="fa-solid fa-cloud-arrow-up" style="font-size: 2.5rem; color: var(--primary); margin-bottom: 10px; display: block;"></i>
        {#if selectedFile}
          <p style="color: var(--success); font-weight: 600; margin: 0;">{selectedFile.name}</p>
        {:else}
          <p style="margin: 0; color: #94a3b8;">Kéo thả file âm thanh .wav hoặc <span style="color: var(--primary);">chọn file</span></p>
        {/if}
        <input type="file" bind:this={fileInput} onchange={handleFileSelect} accept=".wav,audio/wav" class="hidden" />
      </button>

      <!-- Trimmer if file loaded -->
      {#if selectedFile}
        <WaveformTrimmer file={selectedFile} onTrimmed={(_, file) => trimmedFile = file} />
      {/if}

      <!-- Metadata fields form -->
      <div style="margin-top: 1.2rem; padding: 1rem; background: rgba(255, 255, 255, 0.03); border-radius: 12px; border: 1px solid rgba(255, 255, 255, 0.08);">
        <h4 style="margin-bottom: 0.8rem; font-size: 0.9rem; color: var(--primary); display: flex; align-items: center; gap: 6px;">
          <i class="fa-solid fa-bookmark"></i> Thông tin nhãn mác giọng mẫu
        </h4>

        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px;">
          {#each fields as field}
            <div>
              <label for="field-{field.key}" style="font-size: 0.85rem; color: #94a3b8; display: block; margin-bottom: 4px;">
                {field.label} {field.required ? '*' : ''}
              </label>
              {#if field.type === 'select' && field.options}
                <select
                  id="field-{field.key}"
                  bind:value={formData[field.key]}
                  style="width: 100%; height: 38px; padding: 0 12px; border-radius: 6px; background: rgba(15,23,42,0.6); border: 1px solid rgba(255,255,255,0.1); color: white;"
                >
                  {#each field.options as opt}
                    <option value={opt}>{opt}</option>
                  {/each}
                </select>
              {:else}
                <input
                  id="field-{field.key}"
                  type="text"
                  bind:value={formData[field.key]}
                  placeholder={field.placeholder || ''}
                  style="width: 100%; height: 38px; padding: 0 12px; border-radius: 6px; background: rgba(15,23,42,0.6); border: 1px solid rgba(255,255,255,0.1); color: white;"
                />
              {/if}
            </div>
          {/each}
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
