<script lang="ts">
  import type { VoiceOption } from '../types';

  import { onMount } from 'svelte';

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
  let containerRef: HTMLDivElement;

  let currentVoice = $derived(voices.find((v) => v.id === selectedVoiceId));

  onMount(() => {
    function handleOutsideClick(e: MouseEvent) {
      if (isOpen && containerRef && !containerRef.contains(e.target as Node)) {
        isOpen = false;
      }
    }
    window.addEventListener('click', handleOutsideClick);
    return () => {
      window.removeEventListener('click', handleOutsideClick);
      audioElement?.pause();
    };
  });

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

<div class="custom-select-wrapper" class:open={isOpen} bind:this={containerRef}>
  <div class="custom-select-trigger" onclick={(e) => { e.stopPropagation(); isOpen = !isOpen; }} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && (isOpen = !isOpen)}>
    <div class="selected-voice-info" style="display: flex; align-items: center; gap: 8px; flex-wrap: wrap;">
      {#if currentVoice}
        <strong style="font-weight: 600; color: white; font-size: 0.95rem;">{currentVoice.name}</strong>
        {#if (currentVoice.descriptions && currentVoice.descriptions.length > 0) || (currentVoice.metadata && Object.keys(currentVoice.metadata).length > 0)}
          <div class="voice-badges" style="display: inline-flex; gap: 4px; align-items: center; flex-wrap: wrap;">
            {#each (currentVoice.descriptions && currentVoice.descriptions.length > 0 ? currentVoice.descriptions : Object.values(currentVoice.metadata || {}).filter((d) => typeof d === 'string' && d.trim() !== '')).slice(0, 3) as desc, idx}
              <span class="badge {BADGE_COLORS[idx % BADGE_COLORS.length]}">{desc}</span>
            {/each}
          </div>
        {/if}
      {:else}
        <span style="color: var(--text-muted); font-size: 0.95rem;">{placeholder}</span>
      {/if}
    </div>
    <div class="select-chevron-icon" style="flex-shrink: 0; margin-left: 8px; font-size: 0.85rem; color: var(--text-muted);">
      <i class="fa-solid fa-chevron-down"></i>
      <span class="css-arrow-down">▼</span>
    </div>
  </div>

  {#if isOpen}
    <div class="custom-options" style="display: block; opacity: 1; visibility: visible;">
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

      {#each voices as v, idx (v.id ? `${v.id}_${idx}` : `${v.name}_${idx}`)}
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
            {#if (v.descriptions && v.descriptions.length > 0) || (v.metadata && Object.keys(v.metadata).length > 0)}
              <div class="voice-badges" style="display: flex; gap: 5px; margin-top: 2px;">
                {#each (v.descriptions && v.descriptions.length > 0 ? v.descriptions : Object.values(v.metadata || {}).filter((d) => typeof d === 'string' && d.trim() !== '')).slice(0, 5) as desc, idx}
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
                title="Preview voice sample"
              >
                <i class="fa-solid {playingSampleUrl === v.sampleUrl ? 'fa-pause' : 'fa-play'}" style="font-size: 0.8rem;"></i>
              </button>
            {/if}

            {#if onDelete && v.deletable}
              <button
                type="button"
                onclick={(e) => handleDelete(e, v)}
                style="background: rgba(239, 68, 68, 0.15); border: 1px solid rgba(239, 68, 68, 0.4); color: #f87171; border-radius: 50%; width: 30px; height: 30px; display: flex; align-items: center; justify-content: center; cursor: pointer; transition: all 0.2s;"
                title="Delete this voice"
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
