<script lang="ts">
  import WaveformTrimmer from './WaveformTrimmer.svelte';
  import { cloneVoice } from '../api';
  import { toast } from '../toast.svelte';
  import type { VoiceMetadataFieldSpec, UniversalManifest, EngineModeSpec } from '../types';
  import { acceptedUploadFormats, maxUploadBytes, maxUploadLabel, supportedFormatsLabel } from '../audioSpec';

  interface Props {
    isOpen: boolean;
    onClose: () => void;
    onSaved: (newVoiceId: string, newVoiceName: string) => void;
    modelId?: string;
    metadataSchema?: VoiceMetadataFieldSpec[];
    manifest?: UniversalManifest | null;
    mode?: EngineModeSpec | null;
  }

  let { isOpen, onClose, onSaved, modelId = '', metadataSchema, manifest = null, mode = null }: Props = $props();

  let fileInput = $state<HTMLInputElement | null>(null);
  let selectedFile = $state<File | null>(null);
  let trimmedFile = $state<File | null>(null);
  let isSaving = $state(false);

  // Fields the engine declares. The fallback exists only for an engine that ships no
  // voice_metadata_schema at all; anything it contains is a guess, so it stays minimal
  // rather than assuming a language or a set of regional accents.
  let fields = $derived.by(() => {
    if (metadataSchema && metadataSchema.length > 0) {
      return metadataSchema;
    }
    return [
      { key: 'name', label: 'Voice Name', type: 'text', required: true, placeholder: 'e.g. Narrator...' }
    ];
  });

  // Seed each field from its own schema: a select starts on its first option, a text field
  // empty. Hardcoding "Male"/"Northern" here sent Vietnamese regional accents to engines
  // that had never heard of them, and overwrote whatever the engine did declare.
  //
  // $derived rather than an $effect that reads formData and writes it back — that version
  // re-ran on its own write, and the resulting loop wedged the modal so its close button
  // never got a chance to respond.
  let defaults = $derived.by(() => {
    const seeded: Record<string, string> = {};
    for (const f of fields) {
      seeded[f.key] = f.type === 'select' ? (f.options?.[0] ?? '') : '';
    }
    return seeded;
  });

  // Only what the user actually typed. Reading falls back to the derived defaults, so
  // switching modes picks up the new schema without stale values from the old one.
  let edited = $state<Record<string, string>>({});
  let formData = $derived({ ...defaults, ...edited });

  // Clear the form whenever the modal closes, so reopening it does not show the previous
  // voice's details. Keyed on isOpen rather than done in onClose, because the parent can
  // also close it by flipping the prop.
  $effect(() => {
    if (!isOpen) {
      edited = {};
      selectedFile = null;
      trimmedFile = null;
    }
  });

  function handleFileSelect(e: Event) {
    const target = e.target as HTMLInputElement;
    if (target.files && target.files.length > 0) {
      const picked = target.files[0];
      const ceiling = maxUploadBytes(manifest, mode);
      if (picked.size > ceiling) {
        toast.show(`File is ${(picked.size / 1048576).toFixed(1)} MB; the engine accepts up to ${maxUploadLabel(manifest, mode)}.`, 'error');
        target.value = '';
        return;
      }
      selectedFile = picked;
      trimmedFile = selectedFile;
      if (!formData['name']) {
        edited = { ...edited, name: picked.name.replace(/\.[^/.]+$/, '') };
      }
    }
  }

  async function handleSaveVoice() {
    const fileToUpload = trimmedFile || selectedFile;
    if (!fileToUpload) {
      toast.show('Please select a reference audio file!', 'error');
      return;
    }
    const nameVal = formData['name'] || '';
    if (!nameVal.trim()) {
      toast.show('Please enter a voice name!', 'error');
      return;
    }

    isSaving = true;
    toast.show('Uploading & extracting voice embeddings...', 'info');

    try {
      // Pass the whole form through: the engine decided which fields exist, so picking a
      // fixed subset here would discard anything it declared beyond gender/region/style.
      const voiceId = await cloneVoice(fileToUpload, nameVal, formData, modelId);
      toast.show('New voice saved successfully!', 'success');
      onSaved(voiceId, nameVal);
      onClose();
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.show('Error saving voice sample: ' + errMsg, 'error');
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
          <i class="fa-solid fa-microphone-lines"></i> Create & Save New Voice (Clone)
        </h3>
        <button onclick={onClose} aria-label="Close modal" style="background: rgba(255,255,255,0.1); color: white; border: none; width: 32px; height: 32px; border-radius: 50%; display: flex; align-items: center; justify-content: center; font-size: 1rem; cursor: pointer;">
          <i class="fa-solid fa-xmark"></i>
        </button>
      </div>

      <!--
        Flex column rather than text-align: the icon is a block element, and a block does
        not centre from its parent's text-align — it needs a margin or a flex parent.
      -->
      <label for="create-voice-file-input" class="upload-drop-zone" style="display: flex; flex-direction: column; align-items: center; gap: 6px; width: 100%; margin-bottom: 1rem; background: none; border: 2px dashed rgba(255,255,255,0.15); border-radius: 12px; padding: 20px; text-align: center; cursor: pointer;">
        <i class="fa-solid fa-cloud-arrow-up" style="font-size: 2.5rem; color: var(--primary);"></i>
        {#if selectedFile}
          <p style="color: var(--success); font-weight: 600; margin: 0;">{selectedFile.name}</p>
        {:else}
          <p style="color: #cbd5e1; margin: 0;">Drag & drop audio file here or <strong style="color: var(--primary);">click to browse</strong></p>
          <span style="font-size: 0.8rem; opacity: 0.7;">Supported formats: {supportedFormatsLabel()} (max {maxUploadLabel(manifest, mode)})</span>
        {/if}
        <input id="create-voice-file-input" type="file" bind:this={fileInput} onchange={handleFileSelect} accept={acceptedUploadFormats()} style="display: none;" />
      </label>

      <!-- Trimmer if file loaded -->
      {#if selectedFile}
        <WaveformTrimmer file={selectedFile} onTrimmed={(_, file) => trimmedFile = file} {manifest} {mode} />
      {/if}

      <!-- Metadata fields form -->
      <div style="margin-top: 1.2rem; padding: 1rem; background: rgba(255, 255, 255, 0.03); border-radius: 12px; border: 1px solid rgba(255, 255, 255, 0.08);">
        <h4 style="margin-bottom: 0.8rem; font-size: 0.9rem; color: var(--primary); display: flex; align-items: center; gap: 6px;">
          <i class="fa-solid fa-bookmark"></i> Voice Metadata & Attributes
        </h4>

        <!--
          auto-fit rather than a fixed two columns: the engine decides how many fields
          exist, and an odd count left the last one stretched across a half-width cell.
        -->
        <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 10px;">
          {#each fields as field (field.key)}
            <div>
              <label for="field-{field.key}" style="font-size: 0.85rem; color: #94a3b8; display: block; margin-bottom: 4px;">
                {field.label} {field.required ? '*' : ''}
              </label>
              {#if field.type === 'select' && field.options}
                <select
                  id="field-{field.key}"
                  value={formData[field.key] ?? ''}
                  onchange={(e) => (edited = { ...edited, [field.key]: e.currentTarget.value })}
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
                  value={formData[field.key] ?? ''}
                  oninput={(e) => (edited = { ...edited, [field.key]: e.currentTarget.value })}
                  placeholder={field.placeholder || ''}
                  style="width: 100%; height: 38px; padding: 0 12px; border-radius: 6px; background: rgba(15,23,42,0.6); border: 1px solid rgba(255,255,255,0.1); color: white;"
                />
              {/if}
            </div>
          {/each}
        </div>
      </div>

      <div style="display: flex; gap: 12px; justify-content: flex-end; margin-top: 20px;">
        <button onclick={onClose} class="btn secondary-btn" type="button"><i class="fa-solid fa-xmark"></i> Cancel</button>
        <button onclick={handleSaveVoice} disabled={isSaving || !selectedFile} class="btn primary-btn">
          <i class="fa-solid fa-cloud-arrow-up"></i> {isSaving ? 'Saving...' : 'Upload & Save Voice'}
        </button>
      </div>
    </div>
  </div>
{/if}
