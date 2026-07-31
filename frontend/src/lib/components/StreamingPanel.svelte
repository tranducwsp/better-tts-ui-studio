<script lang="ts">
  import { onMount } from 'svelte';
  import { synthesize, subscribeTaskStream } from '../api';
  import { toast } from '../toast.svelte';

  export interface ChunkState {
    index: number;
    text: string;
    status: 'pending' | 'retrying' | 'ready' | 'playing' | 'error';
    blob: Blob | null;
    blobUrl: string | null;
  }

  interface Props {
    text: string;
    engine: string;
    voice: string;
    speed: number;
    reloadedJob?: any | null;
    onClose?: () => void;
  }

  let { text, engine, voice, speed, reloadedJob = null, onClose }: Props = $props();

  let chunks = $state<ChunkState[]>([]);
  let currentPlayIndex = $state<number>(-1);
  let totalRetries = $state<number>(0);
  let statusBadge = $state<string>('Initializing...');
  let isCompleted = $state(false);
  let isCancelled = $state(false);
  let currentJobId = $state<string>('');
  let audioElement: HTMLAudioElement;
  let chunkListContainer: HTMLDivElement;

  let readyCount = $derived(chunks.filter((c) => c.status === 'ready' || c.status === 'playing').length);
  let currentChunk = $derived(currentPlayIndex >= 0 ? chunks[currentPlayIndex] : null);

  onMount(() => {
    startStreamingJob();

    if (chunkListContainer) {
      const handleWheel = (evt: WheelEvent) => {
        if (evt.deltaY !== 0) {
          evt.preventDefault();
          chunkListContainer.scrollLeft += evt.deltaY;
        }
      };
      chunkListContainer.addEventListener('wheel', handleWheel, { passive: false });
      return () => chunkListContainer.removeEventListener('wheel', handleWheel);
    }
  });

  function splitTextIntoChunks(rawText: string, minSize = 1000, maxSize = 2000): string[] {
    if (!rawText || rawText.length <= minSize) return [rawText];
    let paragraphs: string[];
    try {
      paragraphs = rawText.split(/\n\n|\.\s*\n/);
    } catch {
      paragraphs = rawText.split('\n');
    }
    const result: string[] = [];
    let current = '';

    for (const p of paragraphs) {
      const cleanP = p.trim();
      if (!cleanP) continue;

      if (cleanP.length > maxSize) {
        const sentences = cleanP.match(/[^.!?]+[.!?]+/g) || [cleanP];
        for (const s of sentences) {
          const cleanS = s.trim();
          if (!cleanS) continue;
          if (current.length + cleanS.length + 1 <= minSize) {
            current += (current ? ' ' : '') + cleanS;
          } else {
            if (current) result.push(current);
            current = cleanS;
          }
        }
      } else {
        if (current.length + cleanP.length + 1 <= minSize) {
          current += (current ? '\n' : '') + cleanP;
        } else {
          if (current) result.push(current);
          current = cleanP;
        }
      }
    }
    if (current) result.push(current);
    return result;
  }

  async function startStreamingJob() {
    const chunkTexts = splitTextIntoChunks(text, 1000, 2000);

    if (reloadedJob && reloadedJob.chunks && reloadedJob.chunks.length > 0) {
      // Restore chunks strictly from reloaded history job
      const dbChunksMap: Record<number, any> = {};
      reloadedJob.chunks.forEach((c: any) => {
        dbChunksMap[c.chunk_index] = c;
      });

      chunks = chunkTexts.map((ctext, i) => {
        const c = dbChunksMap[i];
        let url = null;
        if (c) {
          if (c.audio_path) {
            url = c.audio_path.startsWith('/') ? c.audio_path : `/${c.audio_path}`;
          } else if (c.task_id || c.id) {
            url = `/api/tasks/${c.task_id || c.id}/audio?format=wav`;
          }
        }

        return {
          index: i,
          text: ctext,
          status: url ? 'ready' : 'pending',
          blob: null,
          blobUrl: url,
        };
      });

      currentPlayIndex = -1;
      currentJobId = reloadedJob.job_id || 'job_' + Date.now();
      statusBadge = 'Loaded from History';
      isCompleted = chunks.every((c) => c.status === 'ready');
      isCancelled = true; // Mark as paused by default when reloaded from history

      // Auto play chunk 0 if ready
      if (chunks.length > 0 && chunks[0].blobUrl) {
        playChunk(0);
      }
      return;
    }

    chunks = chunkTexts.map((ctext, i) => ({
      index: i,
      text: ctext,
      status: 'pending',
      blob: null,
      blobUrl: null,
    }));

    currentPlayIndex = -1;
    totalRetries = 0;
    isCompleted = false;
    isCancelled = false;
    currentJobId = 'job_' + Date.now();

    // Init Job History API
    try {
      await fetch('/api/jobs/init', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          job_id: currentJobId,
          engine: engine,
          voice: voice,
          speed: speed,
          total_chunks: chunks.length,
          text: text,
        }),
        credentials: 'include',
      });
    } catch (e) {}

    generateChunksLoop();
  }

  async function generateChunksLoop() {
    for (let i = 0; i < chunks.length; i++) {
      if (isCancelled) break;
      const item = chunks[i];

      // Skip already ready chunks when reloaded
      if (item.status === 'ready') continue;

      statusBadge = `Generating Chunk ${i + 1}/${chunks.length}...`;

      let success = false;
      let lastError = null;

      for (let attempt = 1; attempt <= 3; attempt++) {
        if (isCancelled) break;

        if (attempt > 1) {
          totalRetries++;
          item.status = 'retrying';
          statusBadge = `Retrying Chunk ${i + 1}/${chunks.length} (Attempt ${attempt}/3)...`;
          await new Promise((r) => setTimeout(r, 2000));
        }

        try {
          const taskId = await synthesize(item.text, voice, speed, engine, currentJobId, i, chunks.length);

          const blob = await new Promise<Blob>((resolve, reject) => {
            subscribeTaskStream(
              taskId,
              () => {},
              (b) => resolve(b),
              (err) => reject(new Error(err))
            );
          });

          if (isCancelled) break;

          item.blob = blob;
          item.blobUrl = URL.createObjectURL(blob);
          item.status = 'ready';
          success = true;

          // Auto-play chunk 1 when ready
          if (currentPlayIndex === -1 && i === 0) {
            playChunk(0);
          }
          break;
        } catch (err: any) {
          lastError = err;
        }
      }

      if (isCancelled) break;

      if (!success) {
        item.status = 'error';
        const errMsg = lastError ? (lastError.message || String(lastError)) : 'Unknown error';
        if (errMsg.includes('Not authenticated') || errMsg.includes('401')) {
          toast.show('Please log in to start AI voice synthesis!', 'error');
          break;
        } else {
          toast.show(`Chunk ${i + 1} failed: ${errMsg}`, 'error');
        }
      }
    }

    if (!isCancelled) {
      isCompleted = true;
      statusBadge = 'Completed!';
      toast.show('All chunks synthesized successfully!', 'success');
    }
  }

  function playChunk(index: number) {
    const item = chunks[index];
    if (!item || !item.blobUrl) return;

    if (currentPlayIndex >= 0 && chunks[currentPlayIndex] && chunks[currentPlayIndex].status === 'playing') {
      chunks[currentPlayIndex].status = 'ready';
    }

    currentPlayIndex = index;
    item.status = 'playing';

    if (audioElement) {
      audioElement.src = item.blobUrl;
      audioElement.play().catch(() => {});
    }
  }

  function handleAudioEnded() {
    if (currentPlayIndex >= 0 && chunks[currentPlayIndex]) {
      chunks[currentPlayIndex].status = 'ready';
    }

    const nextIndex = currentPlayIndex + 1;
    if (nextIndex < chunks.length && chunks[nextIndex] && chunks[nextIndex].blobUrl) {
      playChunk(nextIndex);
    }
  }

  function handleCancel() {
    isCancelled = true;
    statusBadge = 'Stream paused';
    toast.show('Playback stream paused', 'info');
  }

  function handleResume() {
    isCancelled = false;
    statusBadge = 'Resuming generation...';
    toast.show('Resuming synthesis...', 'info');
    generateChunksLoop();
  }

  function downloadCombinedAudio() {
    toast.show('Combining all chunk audio files...', 'info');
    const validBlobs = chunks.filter((c) => c.blob).map((c) => c.blob as Blob);
    if (validBlobs.length === 0) return;
    const finalBlob = new Blob(validBlobs, { type: 'audio/wav' });
    const url = URL.createObjectURL(finalBlob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'voice_full_stream.wav';
    a.click();
  }
</script>

<div class="glass-panel" style="margin-top: 1.5rem; background: rgba(15, 23, 42, 0.85); border: 1px solid var(--primary); box-shadow: 0 10px 30px rgba(99, 102, 241, 0.2);">
  <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.8rem;">
    <h3 style="color: var(--primary); font-size: 1.1rem; margin: 0; display: flex; align-items: center; gap: 8px;">
      <i class="fa-solid fa-compact-disc fa-spin"></i> Audio Generation & Streaming Progress
    </h3>
    <span style="font-size: 0.85rem; padding: 4px 12px; border-radius: 12px; background: rgba(99, 102, 241, 0.2); color: #a5b4fc; font-weight: 500;">
      {statusBadge}
    </span>
  </div>

  <!-- Row 1: Chunk selector buttons -->
  <div style="font-size: 0.85rem; color: var(--text-muted); margin-bottom: 6px; display: flex; justify-content: space-between;">
    <span><i class="fa-solid fa-list-ol"></i> Chunk list (Scroll horizontally or click to jump):</span>
    <div>
      {#if totalRetries > 0}
        <span style="color: #a855f7; font-weight: 600; margin-right: 10px;">
          <i class="fa-solid fa-rotate-right"></i> {totalRetries} retries
        </span>
      {/if}
      <span style="color: #f59e0b; font-weight: 600;">{readyCount} / {chunks.length}</span>
    </div>
  </div>

  <div bind:this={chunkListContainer} style="display: flex; gap: 8px; overflow-x: auto; padding: 6px 4px 12px 4px; max-width: 100%;">
    {#each chunks as c, i (c.index)}
      <button
        type="button"
        onclick={() => playChunk(i)}
        disabled={!c.blobUrl}
        style="padding: 6px 14px; font-size: 0.85em; white-space: nowrap; flex-shrink: 0; border-radius: 6px; transition: all 0.2s ease; cursor: pointer; border: 1px solid;
          background: {c.status === 'playing' ? '#10b981' : (c.status === 'ready' ? '#f59e0b' : (c.status === 'retrying' ? '#a855f7' : (c.status === 'error' ? '#ef4444' : 'rgba(255,255,255,0.06)')))};
          color: {c.status === 'playing' || c.status === 'retrying' || c.status === 'error' ? '#ffffff' : (c.status === 'ready' ? '#1e293b' : '#94a3b8')};
          border-color: {c.status === 'playing' ? '#059669' : (c.status === 'ready' ? '#d97706' : (c.status === 'retrying' ? '#7e22ce' : (c.status === 'error' ? '#b91c1c' : 'rgba(255,255,255,0.1)')))};
        "
        title="Click to play Chunk {i + 1}"
      >
        Chunk {i + 1}: {c.text.substring(0, 20).replace(/\n/g, ' ')}...
      </button>
    {/each}
  </div>

  <!-- Row 2: Audio Player & Action Controls -->
  <div style="margin-top: 8px; background: rgba(0,0,0,0.3); padding: 14px; border-radius: 10px; text-align: center;">
    <p style="font-size: 0.9rem; color: #fff; margin-bottom: 8px; text-align: left; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
      <i class="fa-solid fa-music" style="color: var(--primary);"></i> Currently playing:
      <span style="color: #94a3b8; font-style: italic;">
        {currentChunk ? `Chunk ${currentChunk.index + 1}: ${currentChunk.text.substring(0, 40)}...` : 'No chunk selected'}
      </span>
    </p>

    <audio
      bind:this={audioElement}
      onended={handleAudioEnded}
      controls
      style="width: 100%; margin-bottom: 10px; outline: none;"
    ></audio>

    <div style="display: flex; gap: 12px; justify-content: center; align-items: center; margin-top: 10px; flex-wrap: wrap;">
      {#if !isCompleted && !isCancelled}
        <button onclick={handleCancel} class="btn" style="padding: 0.5rem 1.2rem; font-size: 0.9rem; background: #ef4444; color: white; border-radius: 8px; font-weight: 500; cursor: pointer; display: flex; align-items: center; gap: 6px;">
          <i class="fa-solid fa-pause"></i> Pause Generation
        </button>
      {/if}

      {#if isCancelled && !isCompleted}
        <button onclick={handleResume} class="btn" style="padding: 0.5rem 1.2rem; font-size: 0.9rem; background: #10b981; color: white; border-radius: 8px; font-weight: 500; cursor: pointer; display: flex; align-items: center; gap: 6px;">
          <i class="fa-solid fa-play"></i> Resume Generation
        </button>
      {/if}

      {#if currentChunk && currentChunk.blobUrl}
        <a href={currentChunk.blobUrl} download="chunk_{currentChunk.index + 1}.wav" class="btn secondary-btn" style="padding: 0.5rem 1.2rem; font-size: 0.9rem; background: #6366f1; color: white; border-radius: 8px; text-decoration: none; display: inline-flex; align-items: center; gap: 6px;">
          <i class="fa-solid fa-download"></i> WAV (Chunk {currentChunk.index + 1})
        </a>
      {/if}

      {#if isCompleted}
        <button onclick={downloadCombinedAudio} class="btn secondary-btn" style="padding: 0.5rem 1.2rem; font-size: 0.9rem; background: #10b981; color: white; border-radius: 8px; display: inline-flex; align-items: center; gap: 6px; cursor: pointer; border: none;">
          <i class="fa-solid fa-download"></i> Download Full WAV
        </button>
      {/if}
    </div>
  </div>
</div>
