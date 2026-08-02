<script lang="ts">
  import { audioBufferToWav } from '../audioWav';
  import { toast } from '../toast.svelte';
  import { referenceAudioSeconds } from '../audioSpec';
  import type { UniversalManifest, EngineModeSpec } from '../types';

  interface Props {
    file: File;
    manifest?: UniversalManifest | null;
    mode?: EngineModeSpec | null;
    onTrimmed: (trimmedBlob: Blob, trimmedFile: File) => void;
  }

  let { file, onTrimmed, manifest = null, mode = null }: Props = $props();

  let canvasElement = $state<HTMLCanvasElement | null>(null);
  let audioBuffer = $state<AudioBuffer | null>(null);
  let duration = $state(0);
  let trimStart = $state(0);
  let trimEnd = $state(0);
  let isDragging = $state(false);
  let previewAudioUrl = $state<string | null>(null);

  // How long a reference clip the engine wants. Engines differ — some need three seconds,
  // some ten — so this comes from audio_spec rather than a constant here.
  let TRIM_LENGTH = $derived(referenceAudioSeconds(manifest, mode));

  let wavePeaks: { min: number; max: number }[] = [];
  let rafPending = false;

  $effect(() => {
    if (file) {
      loadAudioFile(file);
    }
  });

  async function loadAudioFile(f: File) {
    try {
      const arrayBuffer = await f.arrayBuffer();
      const AudioCtx = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
      const audioCtx = new AudioCtx();
      audioBuffer = await audioCtx.decodeAudioData(arrayBuffer);
      duration = audioBuffer.duration;
      trimStart = 0;
      trimEnd = Math.min(TRIM_LENGTH, duration);
      previewAudioUrl = URL.createObjectURL(f);

      // Precalculate waveform peaks ONCE for 600px width
      if (canvasElement) {
        const width = canvasElement.width;
        const data = audioBuffer.getChannelData(0);
        const step = Math.ceil(data.length / width);
        wavePeaks = [];
        for (let i = 0; i < width; i++) {
          let min = 1.0;
          let max = -1.0;
          for (let j = 0; j < step; j++) {
            const datum = data[i * step + j] || 0;
            if (datum < min) min = datum;
            if (datum > max) max = datum;
          }
          wavePeaks.push({ min, max });
        }
      }

      drawWaveform();
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.show('Error loading audio file: ' + errMsg, 'error');
    }
  }

  function drawWaveform() {
    if (!canvasElement || !audioBuffer) return;
    const ctx = canvasElement.getContext('2d');
    if (!ctx) return;

    const width = canvasElement.width;
    const height = canvasElement.height;
    const amp = height / 2;

    ctx.clearRect(0, 0, width, height);

    // Draw precalculated waves
    ctx.fillStyle = 'rgba(99, 102, 241, 0.5)';
    for (let i = 0; i < wavePeaks.length; i++) {
      const peak = wavePeaks[i];
      ctx.fillRect(i, (1 + peak.min) * amp, 1, Math.max(1, (peak.max - peak.min) * amp));
    }

    // Draw Selection Overlay
    const startX = (trimStart / duration) * width;
    const endX = (trimEnd / duration) * width;

    ctx.fillStyle = 'rgba(0, 0, 0, 0.6)';
    ctx.fillRect(0, 0, startX, height);
    ctx.fillRect(endX, 0, width - endX, height);

    // Draw Edges
    ctx.fillStyle = '#10b981';
    ctx.fillRect(startX - 2, 0, 4, height);
    ctx.fillRect(endX - 2, 0, 4, height);
  }

  function scheduleDraw() {
    if (!rafPending) {
      rafPending = true;
      requestAnimationFrame(() => {
        drawWaveform();
        rafPending = false;
      });
    }
  }

  function updateWindowPos(clientX: number) {
    if (!canvasElement || !duration) return;
    const rect = canvasElement.getBoundingClientRect();
    const scaleX = canvasElement.width / rect.width;
    const x = (clientX - rect.left) * scaleX;
    const timeClicked = (x / canvasElement.width) * duration;

    let start = timeClicked - TRIM_LENGTH / 2;
    let end = timeClicked + TRIM_LENGTH / 2;

    if (start < 0) {
      start = 0;
      end = Math.min(TRIM_LENGTH, duration);
    }
    if (end > duration) {
      end = duration;
      start = Math.max(0, duration - TRIM_LENGTH);
    }

    trimStart = start;
    trimEnd = end;
    scheduleDraw();
  }

  function handleMouseDown(e: MouseEvent) {
    isDragging = true;
    updateWindowPos(e.clientX);
  }

  function handleMouseMove(e: MouseEvent) {
    if (isDragging) {
      updateWindowPos(e.clientX);
    }
  }

  function handleMouseUp() {
    isDragging = false;
  }

  function handleTouchStart(e: TouchEvent) {
    if (e.touches.length > 0) {
      isDragging = true;
      updateWindowPos(e.touches[0].clientX);
    }
  }

  function handleTouchMove(e: TouchEvent) {
    if (isDragging && e.touches.length > 0) {
      updateWindowPos(e.touches[0].clientX);
    }
  }

  function handleTouchEnd() {
    isDragging = false;
  }

  function handleTrimOnly() {
    if (!audioBuffer) return;
    const blob = audioBufferToWav(audioBuffer, trimStart, trimEnd);
    const newFileName = file.name.replace(/\.[^/.]+$/, '') + '_trimmed.wav';
    const trimmedFile = new File([blob], newFileName, { type: 'audio/wav' });
    previewAudioUrl = URL.createObjectURL(blob);
    onTrimmed(blob, trimmedFile);
    toast.show(`${TRIM_LENGTH}s audio segment trimmed!`, 'success');
  }
</script>

<div style="background: rgba(15, 23, 42, 0.4); padding: 1rem; border-radius: 12px; margin-top: 1rem; border: 1px dashed var(--primary);">
  <h4 style="margin-bottom: 0.5rem; color: #fff; font-size: 0.95rem;">
    <i class="fa-solid fa-scissors" style="color: var(--primary);"></i> Select optimal {TRIM_LENGTH}s audio segment
  </h4>

  <div style="margin-bottom: 0.5rem; position: relative;">
    <canvas
      bind:this={canvasElement}
      width="600"
      height="100"
      onmousedown={handleMouseDown}
      onmousemove={handleMouseMove}
      onmouseup={handleMouseUp}
      onmouseleave={handleMouseUp}
      ontouchstart={handleTouchStart}
      ontouchmove={handleTouchMove}
      ontouchend={handleTouchEnd}
      style="width: 100%; height: 100px; background: #0f172a; border-radius: 8px; cursor: pointer; user-select: none; touch-action: none;"
    ></canvas>
    <p style="font-size: 0.8rem; color: #94a3b8; text-align: center; margin-top: 5px;">
      Click or drag on the waveform to select the best {TRIM_LENGTH}-second range.
    </p>
  </div>

  <div style="display: flex; justify-content: space-between; margin-bottom: 0.8rem; font-size: 0.85rem; color: var(--text-color);">
    <div><span style="color: var(--primary);">Start:</span> <strong>{trimStart.toFixed(1)}s</strong></div>
    <div><span style="color: var(--primary);">Duration:</span> <strong>{(trimEnd - trimStart).toFixed(1)}s</strong></div>
    <div><span style="color: var(--primary);">End:</span> <strong>{trimEnd.toFixed(1)}s</strong></div>
  </div>

  {#if previewAudioUrl}
    <div style="margin-bottom: 0.8rem;">
      <audio controls src={previewAudioUrl} style="width: 100%; height: 36px; border-radius: 8px;"></audio>
    </div>
  {/if}

  <div style="display: flex; gap: 10px;">
    <button onclick={handleTrimOnly} class="btn secondary-btn" type="button" style="font-size: 0.9rem; padding: 8px 14px;">
      <i class="fa-solid fa-scissors"></i> Trim &amp; Preview {TRIM_LENGTH}s
    </button>
  </div>
</div>
