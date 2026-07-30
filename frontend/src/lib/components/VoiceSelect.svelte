<script lang="ts">
  import type { VoiceOption } from '../types';

  interface Props {
    voices: VoiceOption[];
    selectedVoiceId: string;
    placeholder?: string;
    onSelect?: (voice: VoiceOption) => void;
    onDelete?: (voice: VoiceOption) => void;
  }

  let { voices = [], selectedVoiceId = $bindable(''), placeholder = 'Select a voice speaker...', onSelect, onDelete }: Props = $props();

  let isOpen = $state(false);
  let playingSampleUrl = $state<string | null>(null);
  let audioElement: HTMLAudioElement;

  let currentVoice = $derived(voices.find((v) => v.id === selectedVoiceId));

  function handleSelect(v: VoiceOption | null) {
    if (v) {
      selectedVoiceId = v.id;
      if (onSelect) onSelect(v);
    } else {
      selectedVoiceId = '';
    }
    isOpen = false;
  }

  function playSample(e: Event, sampleUrl?: string) {
    e.stopPropagation();
    if (!sampleUrl) return;
    if (playingSampleUrl === sampleUrl) {
      audioElement?.pause();
      playingSampleUrl = null;
    } else {
      playingSampleUrl = sampleUrl;
      if (audioElement) {
        audioElement.src = sampleUrl;
        audioElement.play();
      }
    }
  }

  function handleDelete(e: Event, v: VoiceOption) {
    e.stopPropagation();
    if (onDelete) {
      onDelete(v);
    }
  }

  const BADGE_COLORS = ['badge-blue', 'badge-purple', 'badge-emerald', 'badge-amber', 'badge-rose'];
</script>

<audio bind:this={audioElement} onended={() => playingSampleUrl = null} class="hidden"></audio>

<div class="custom-select-wrapper" class:open={isOpen}>
  <div class="custom-select-trigger" onclick={() => isOpen = !isOpen} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && (isOpen = !isOpen)}>
    <div class="selected-voice-info" style="display: flex; align-items: center; gap: 10px; flex-wrap: wrap;">
      {#if currentVoice}
        <strong style="font-weight: 600; color: white; font-size: 0.95rem;">{currentVoice.name}</strong>
        {#if currentVoice.descriptions && currentVoice.descriptions.length > 0}
          <div class="voice-badges" style="display: inline-flex; gap: 5px; align-items: center; margin-top: 0;">
            {#each currentVoice.descriptions.slice(0, 5) as desc, idx}
              <span class="badge {BADGE_COLORS[idx % BADGE_COLORS.length]}">{desc}</span>
            {/each}
          </div>
        {/if}
      {:else}
        <span style="color: var(--text-muted); font-size: 0.95rem;">{placeholder}</span>
      {/if}
    </div>
    <i class="fa-solid fa-chevron-down"></i>
  </div>

  {#if isOpen}
    <div class="custom-options">
      {#if placeholder}
        <div
          class="custom-option"
          class:selected={!selectedVoiceId}
          onclick={() => handleSelect(null)}
          role="button"
          tabindex="0"
          onkeydown={(e) => e.key === 'Enter' && handleSelect(null)}
        >
          <div class="voice-title" style="color: var(--text-muted); font-size: 0.95rem;">
            {placeholder}
          </div>
        </div>
      {/if}

      {#each voices as v (v.id)}
        <div
          class="custom-option"
          class:selected={v.id === selectedVoiceId}
          onclick={() => handleSelect(v)}
          role="button"
          tabindex="0"
          onkeydown={(e) => e.key === 'Enter' && handleSelect(v)}
          style="display: flex; justify-content: space-between; align-items: center; padding: 10px 14px;"
        >
          <div style="display: flex; flex-direction: column; gap: 4px;">
            <div class="voice-title" style="font-weight: 600; color: white;">{v.name}</div>
            {#if v.descriptions && v.descriptions.length > 0}
              <div class="voice-badges" style="display: flex; gap: 5px; margin-top: 2px;">
                {#each v.descriptions.slice(0, 5) as desc, idx}
                  <span class="badge {BADGE_COLORS[idx % BADGE_COLORS.length]}">{desc}</span>
                {/each}
              </div>
            {/if}
          </div>

          <div style="display: flex; gap: 8px; align-items: center; margin-left: 12px; flex-shrink: 0;">
            {#if v.sampleUrl}
              <button
                type="button"
                onclick={(e) => playSample(e, v.sampleUrl)}
                style="background: rgba(99, 102, 241, 0.2); border: 1px solid var(--primary); color: #a5b4fc; border-radius: 50%; width: 30px; height: 30px; display: flex; align-items: center; justify-content: center; cursor: pointer;"
                title="Nghe thử mẫu giọng"
              >
                <i class="fa-solid {playingSampleUrl === v.sampleUrl ? 'fa-pause' : 'fa-play'}" style="font-size: 0.8rem;"></i>
              </button>
            {/if}

            {#if onDelete}
              <button
                type="button"
                onclick={(e) => handleDelete(e, v)}
                style="background: rgba(239, 68, 68, 0.15); border: 1px solid rgba(239, 68, 68, 0.4); color: #f87171; border-radius: 50%; width: 30px; height: 30px; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: all 0.2s;"
                title="Xóa giọng mẫu này"
              >
                <i class="fa-solid fa-trash-can" style="font-size: 0.8rem;"></i>
              </button>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
