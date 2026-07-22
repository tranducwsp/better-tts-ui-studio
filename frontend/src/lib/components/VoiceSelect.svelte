<script lang="ts">
  import type { VoiceOption } from '../types';

  interface Props {
    voices: VoiceOption[];
    selectedVoiceId: string;
    placeholder?: string;
    onSelect?: (voice: VoiceOption) => void;
    onDelete?: (voice: VoiceOption) => void;
  }

  let { voices = [], selectedVoiceId = $bindable(''), placeholder = 'Đang tải danh sách giọng...', onSelect, onDelete }: Props = $props();

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
</script>

<audio bind:this={audioElement} onended={() => playingSampleUrl = null} class="hidden"></audio>

<div class="custom-select-wrapper" class:open={isOpen}>
  <div class="custom-select-trigger" onclick={() => isOpen = !isOpen} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && (isOpen = !isOpen)}>
    <div class="selected-voice-info" style="display: flex; align-items: center; gap: 10px; flex-wrap: wrap;">
      {#if currentVoice}
        <strong style="font-weight: 600; color: white; font-size: 0.95rem;">{currentVoice.name}</strong>
        <div class="voice-badges" style="display: inline-flex; gap: 5px; align-items: center; margin-top: 0;">
          {#if currentVoice.gender}
            <span class="badge {currentVoice.gender === 'Nam' ? 'gender-nam' : 'gender-nu'}">{currentVoice.gender}</span>
          {/if}
          {#if currentVoice.region}
            <span class="badge region">{currentVoice.region}</span>
          {/if}
          {#if currentVoice.description}
            <span class="badge style">{currentVoice.description}</span>
          {/if}
        </div>
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
            {#if v.gender || v.region || v.description}
              <div class="voice-badges" style="display: flex; gap: 5px; margin-top: 2px;">
                {#if v.gender}<span class="badge {v.gender === 'Nam' ? 'gender-nam' : 'gender-nu'}">{v.gender}</span>{/if}
                {#if v.region}<span class="badge region">{v.region}</span>{/if}
                {#if v.description}<span class="badge style">{v.description}</span>{/if}
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
