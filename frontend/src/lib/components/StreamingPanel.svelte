<script lang="ts">
  import { onMount } from 'svelte';
  import { synthesize, subscribeTaskStream, authFetch } from '../api';
  import { splitIntoChunks } from '../textLimits';
  import { combinedDownloadFormat, defaultFormat, defaultMimeType, downloadFormats } from '../audioSpec';
  import { toast } from '../toast.svelte';
  import type { JobDetailResponse, ChunkItemResponse, UniversalManifest } from '../types';

  export interface ChunkState {
    index: number;
    text: string;
    status: 'pending' | 'retrying' | 'ready' | 'playing' | 'error';
    blob: Blob | null;
    blobUrl: string | null;
    /** Backend task id, used to fetch alternate formats such as MP3. */
    taskId: string | null;
  }

  interface Props {
    text: string;
    engine: string;
    voice: string;
    speed: number;
    pitch?: number;
    emotion?: string;
    manifest?: UniversalManifest | null;
    reloadedJob?: JobDetailResponse | null;
    onClose?: () => void;
  }

  let { text, engine, voice, speed, pitch, emotion, manifest = null, reloadedJob = null, onClose }: Props = $props();


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

  // D1: Khi component unmount (đóng panel), dừng vòng lặp generation và thu hồi blob URL.
  // Không có bước này, 50 chunk đóng sau 5 → 45 chunk âm thầm chạy tiếp tốn GPU, mỗi chunk
  // tạo thêm một dòng lịch sử vô chủ, và blob URL rò cho đến khi trang bị nạp lại.
  onMount(() => {
    return () => {
      isCancelled = true;
      chunks.forEach((c) => {
        if (c.blobUrl?.startsWith('blob:')) URL.revokeObjectURL(c.blobUrl);
      });
    };
  });

  async function startStreamingJob() {
    const chunkTexts = splitIntoChunks(text, manifest);

    if (reloadedJob && reloadedJob.chunks && reloadedJob.chunks.length > 0) {
      // Restore chunks strictly from reloaded history job
      const dbChunksMap: Record<number, ChunkItemResponse> = {};
      reloadedJob.chunks.forEach((c) => {
        dbChunksMap[c.chunk_index] = c;
      });

      chunks = chunkTexts.map((ctext, i) => {
        const c = dbChunksMap[i];
        // Luôn đi qua /api/tasks/{id}/audio.
        //
        // Trước đây chunk mang audio_path và chỗ này ghép nó thành URL trực tiếp. Đó là khoá
        // nội bộ của kho, nên nó chỉ có cơ hội hoạt động khi backend tình cờ phục vụ tĩnh
        // đúng thư mục đó — và với S3 thì không bao giờ. Endpoint API kiểm quyền sở hữu mỗi
        // lần gọi, còn một URL trỏ thẳng vào kho thì không.
        let url = null;
        if (c?.task_id) {
          url = `/api/tasks/${c.task_id}/audio?format=${defaultFormat(manifest, engine)}`;
        }

        return {
          index: i,
          text: ctext,
          status: url ? 'ready' : 'pending',
          blob: null,
          blobUrl: url,
          taskId: c?.task_id || null,
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
      taskId: null,
    }));

    currentPlayIndex = -1;
    totalRetries = 0;
    isCompleted = false;
    isCancelled = false;
    currentJobId = 'job_' + Date.now();

    // Init Job History API
    try {
      await authFetch('/api/jobs/init', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          job_id: currentJobId,
          engine: engine,
          voice: voice,
          speed: speed,
          pitch: pitch ?? null,
          emotion: emotion || null,
          total_chunks: chunks.length,
          text: text,
        }),
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
          const taskId = await synthesize(item.text, voice, speed, engine, {
            jobId: currentJobId,
            chunkIndex: i,
            totalChunks: chunks.length,
            pitch,
            emotion,
          });

          item.taskId = taskId;

          const blob = await new Promise<Blob>((resolve, reject) => {
            subscribeTaskStream(
              taskId,
              () => {},
              (b) => resolve(b),
              (err) => reject(new Error(err)),
              defaultFmt
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
        } catch (err: unknown) {
          lastError = err as Error;
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

  // Download buttons follow what the engine declares, not a hardcoded WAV assumption.
  let defaultFmt = $derived(defaultFormat(manifest, engine));
  let chunkFormats = $derived(downloadFormats(manifest, engine));
  let combinedFormat = $derived(combinedDownloadFormat(manifest, engine));
  let selectedFormat = $state('');
  let isCombining = $state(false);

  // Default the picker to the engine's own format once the manifest is known.
  $effect(() => {
    if (!selectedFormat || !chunkFormats.includes(selectedFormat)) {
      selectedFormat = defaultFmt;
    }
  });

  // The default format is already in memory as a blob; anything else is transcoded by the
  // backend on request.
  let chunkDownloadHref = $derived.by(() => {
    if (!currentChunk) return null;
    if (selectedFormat === defaultFmt) return currentChunk.blobUrl;
    return currentChunk.taskId
      ? `/api/tasks/${currentChunk.taskId}/audio?format=${selectedFormat}`
      : null;
  });

  async function downloadCombinedAudio() {
    if (!combinedFormat || isCombining) return;

    const ready = chunks.filter((c) => c.taskId);
    if (ready.length === 0) {
      toast.show('No finished chunks to combine yet.', 'error');
      return;
    }

    isCombining = true;
    toast.show(`Fetching ${ready.length} chunks as ${combinedFormat.toUpperCase()}...`, 'info');
    try {
      // Fetch each chunk in the combined format rather than concatenating the WAV blobs we
      // already hold: appending WAV files leaves the first header in place, so players see
      // only the first chunk's duration.
      const parts = await Promise.all(
        ready.map(async (c) => {
          const res = await authFetch(`/api/tasks/${c.taskId}/audio?format=${combinedFormat}`);
          if (!res.ok) throw new Error(`Chunk ${c.index + 1} failed (${res.status})`);
          return res.blob();
        })
      );

      const finalBlob = new Blob(parts, { type: defaultMimeType(manifest, engine) });
      const url = URL.createObjectURL(finalBlob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `voice_full_stream.${combinedFormat}`;
      a.click();
      URL.revokeObjectURL(url);
      toast.show('Combined audio ready!', 'success');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      toast.show('Could not combine audio: ' + msg, 'error');
    } finally {
      isCombining = false;
    }
  }
</script>

<div class="glass-panel" style="margin-top: 1.5rem; background: rgba(15, 23, 42, 0.85); border: 1px solid var(--primary); box-shadow: 0 10px 30px rgba(99, 102, 241, 0.2);">
  <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.8rem;">
    <h3 style="color: var(--primary); font-size: 1.1rem; margin: 0; display: flex; align-items: center; gap: 8px;">
      <i class="fa-solid fa-compact-disc fa-spin"></i> Audio Generation & Streaming Progress
    </h3>
    <div style="display: flex; align-items: center; gap: 10px;">
      <span style="font-size: 0.85rem; padding: 4px 12px; border-radius: 12px; background: rgba(99, 102, 241, 0.2); color: #a5b4fc; font-weight: 500;">
        {statusBadge}
      </span>
      <!-- Closing releases the text lock in the parent, so the user can edit and re-run. -->
      <button
        onclick={() => onClose?.()}
        type="button"
        title="Close panel and unlock the text"
        aria-label="Close streaming panel"
        style="background: rgba(255,255,255,0.08); border: 1px solid rgba(255,255,255,0.15); color: #cbd5e1; width: 28px; height: 28px; border-radius: 6px; cursor: pointer; display: flex; align-items: center; justify-content: center; flex-shrink: 0;"
      >
        <i class="fa-solid fa-xmark"></i>
      </button>
    </div>
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
        <button onclick={handleCancel} style="height: 36px; padding: 0 1rem; font-size: 0.85rem; font-weight: 500; line-height: 1; background: #ef4444; color: white; border: none; border-radius: 8px; cursor: pointer; display: inline-flex; align-items: center; gap: 6px; white-space: nowrap;">
          <i class="fa-solid fa-pause"></i> Pause Generation
        </button>
      {/if}

      {#if isCancelled && !isCompleted}
        <button onclick={handleResume} style="height: 36px; padding: 0 1rem; font-size: 0.85rem; font-weight: 500; line-height: 1; background: #10b981; color: white; border: none; border-radius: 8px; cursor: pointer; display: inline-flex; align-items: center; gap: 6px; white-space: nowrap;">
          <i class="fa-solid fa-play"></i> Resume Generation
        </button>
      {/if}

      <!--
        One select rather than one button per format: an engine may declare half a dozen,
        and a row of download buttons would crowd out the transport controls.

        Deliberately not using .btn here — that class carries padding: 1rem 1.5rem and
        flex: 1, which stretches the pair out of line with the transport buttons. Both
        halves set an explicit height so the select and the link match exactly.
      -->
      {#if currentChunk && chunkDownloadHref}
        <div style="display: inline-flex; align-items: center; height: 36px;">
          <select
            bind:value={selectedFormat}
            aria-label="Download format for the current chunk"
            style="height: 100%; padding: 0 0.55rem; font-size: 0.85rem; font-weight: 500; line-height: 1; background: rgba(255,255,255,0.08); color: white; border: 1px solid rgba(255,255,255,0.2); border-right: none; border-radius: 8px 0 0 8px; cursor: pointer; appearance: none; text-align: center;"
          >
            {#each chunkFormats as fmt (fmt)}
              <option value={fmt} style="background: #1e293b;">{fmt.toUpperCase()}</option>
            {/each}
          </select>
          <a
            href={chunkDownloadHref}
            download="chunk_{currentChunk.index + 1}.{selectedFormat}"
            style="height: 100%; padding: 0 1rem; font-size: 0.85rem; font-weight: 500; line-height: 1; background: #6366f1; color: white; border: 1px solid #6366f1; border-radius: 0 8px 8px 0; text-decoration: none; display: inline-flex; align-items: center; gap: 6px; white-space: nowrap;"
          >
            <i class="fa-solid fa-download"></i> Chunk {currentChunk.index + 1}
          </a>
        </div>
      {/if}

      <!--
        Combined download is MP3-only: concatenating WAV blobs yields a file whose header
        claims the length of the first chunk, so most players stop there.
      -->
      {#if isCompleted && combinedFormat}
        <button
          onclick={downloadCombinedAudio}
          disabled={isCombining}
          style="height: 36px; padding: 0 1rem; font-size: 0.85rem; font-weight: 500; line-height: 1; background: #10b981; color: white; border: none; border-radius: 8px; display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; cursor: {isCombining ? 'wait' : 'pointer'}; opacity: {isCombining ? 0.6 : 1};"
        >
          <i class="fa-solid {isCombining ? 'fa-spinner fa-spin' : 'fa-download'}"></i>
          {isCombining ? 'Preparing...' : `Full ${combinedFormat.toUpperCase()}`}
        </button>
      {/if}
    </div>
  </div>
</div>
