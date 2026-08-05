<script lang="ts">
  import { onMount } from 'svelte';
  import Header from './lib/components/Header.svelte';
  import TextInputPanel from './lib/components/TextInputPanel.svelte';
  import GenericEnginePanel from './lib/components/GenericEnginePanel.svelte';
  import StreamingPanel from './lib/components/StreamingPanel.svelte';
  import HistoryModal from './lib/components/HistoryModal.svelte';
  import AuthModal from './lib/components/AuthModal.svelte';
  import AdminModal from './lib/components/AdminModal.svelte';
  import Toast from './lib/components/Toast.svelte';
  import { fetchManifest, checkCurrentUser, logout, type UserResponse } from './lib/api';
  import { toast } from './lib/toast.svelte';
  import type { UniversalManifest, JobDetailResponse } from './lib/types';

  interface Props {
    initialManifest?: UniversalManifest | null;
  }

  let { initialManifest = null }: Props = $props();

  // Svelte 5 states using runes
  // Empty until the manifest names a mode. The effect below picks the first one the engine
  // declares; seeding this with a guess like "fast" would render a tab for a mode the
  // engine may not have, for the frame before the correction lands.
  let activeTab = $state<string>('');
  let mainText = $state(
    'Hello! Welcome to AI Voice Studio. Experience high-quality neural voice synthesis!'
  );
  let isReadOnly = $state(false);
  let reloadedJob = $state<JobDetailResponse | null>(null);
  let currentUser = $state<UserResponse | null>(null);

  // Manifest fetched at runtime. Falls back to the SSG-injected prop until it resolves,
  // so the prerendered markup and the first hydrated render agree.
  let fetchedManifest = $state<UniversalManifest | null>(null);
  let manifest = $derived(fetchedManifest ?? initialManifest);

  // UI mode is decided by the engine and published in the manifest. Only "fast" means
  // anything to CSS — app.css keys ~11 rules off body[data-mode="fast"] to drop blobs,
  // backdrop filters and icons — so beauty is expressed by the attribute being absent,
  // exactly as prerender.js writes it. Setting data-mode="beauty" would leave the hydrated
  // DOM differing from the prerendered HTML for no benefit.
  //
  // Guarded on document because this component is also rendered by svelte/server during
  // SSG, where $effect does not run today but nothing guarantees that.
  let uiMode = $derived(manifest?.ui_schema?.ui_mode ?? 'beauty');
  $effect(() => {
    if (typeof document === 'undefined') return;
    if (uiMode === 'fast') {
      document.body.setAttribute('data-mode', 'fast');
    } else {
      document.body.removeAttribute('data-mode');
    }
  });

  // Keep the active tab valid whenever the manifest's mode list changes.
  $effect(() => {
    if (activeModes.length > 0 && !activeModes.some((m) => m.id === activeTab)) {
      activeTab = activeModes[0].id;
    }
  });

  // Dynamic tab ordering derived from manifest ui_schema
  let activeModes = $derived.by(() => {
    if (!manifest) return [];
    const sortOrder = manifest.ui_schema?.model_sort;
    const allModes = manifest.supported_modes || [];
    const cleanName = (name: string) => name.replace(/\s+(Engine|Model)$/i, '');
    if (sortOrder && sortOrder.length > 0) {
      return sortOrder.map((id) => {
        const modeSpec = allModes.find((m) => m.id === id);
        return {
          id,
          name: modeSpec ? cleanName(modeSpec.name) : cleanName(id.toUpperCase())
        };
      });
    }
    return allModes.map((m) => ({
      ...m,
      name: cleanName(m.name)
    }));
  });

  // Derived current mode specification for active tab
  let currentModeSpec = $derived.by(() => {
    const found = (manifest?.supported_modes || []).find((m) => m.id === activeTab);
    return (
      found || {
        id: activeTab,
        name: activeTab.toUpperCase(),
        description: 'Universal Neural Voice Engine'
      }
    );
  });

  // Modals
  let isAuthOpen = $state(false);
  let isHistoryOpen = $state(false);
  let isAdminOpen = $state(false);
  let adminTargetUserId = $state<string | null>(null);
  let adminTargetUsername = $state<string | null>(null);

  // Standardized Runtime Lifecycle: Check auth & refresh manifest if not pre-rendered
  onMount(() => {
    checkCurrentUser().then((user) => {
      currentUser = user;
    });
    if (!initialManifest) {
      fetchManifest().then((m) => {
        if (m) fetchedManifest = m;
      });
    }
  });

  function handleAuthSuccess(user: UserResponse) {
    currentUser = user;
    isAuthOpen = false;
  }

  async function handleLogout() {
    await logout();
    currentUser = null;
    isAuthOpen = true;
    toast.show('Logged out successfully', 'info');
  }

  function selectTab(tab: string) {
    activeTab = tab;
  }

  function handleOpenMyHistory() {
    adminTargetUserId = null;
    adminTargetUsername = null;
    isHistoryOpen = true;
  }

  function handleViewUserHistory(userId: string, username: string) {
    adminTargetUserId = userId;
    adminTargetUsername = username;
    isHistoryOpen = true;
  }

  function handleReloadJob(job: JobDetailResponse) {
    mainText = job.text;
    isReadOnly = true;
    reloadedJob = job;
    if (job.engine) {
      activeTab = job.engine;
    }
    isHistoryOpen = false;
    toast.show(`Reloaded task ${job.job_id ? job.job_id.substring(0, 8) : ''}!`, 'success');
  }

  /**
   * A streaming job in flight. Non-null means the panel is open; the id changes on every
   * start so the panel remounts and re-chunks the current text.
   */
  let streamingJob = $state<{
    id: number;
    engine: string;
    voice: string;
    speed: number;
    pitch?: number;
    emotion?: string;
  } | null>(null);

  let streamingJobCounter = 0;

  // Accordion state for the two panels above the streaming panel. Both collapse when a
  // job starts, as toggleTextPanel(false)/toggleSettingsPanel(false) did in the original.
  let isTextCollapsed = $state(false);
  let isSettingsCollapsed = $state(false);

  function handleStartStreaming(params: {
    engine: string;
    voice: string;
    speed: number;
    pitch?: number;
    emotion?: string;
  }) {
    // Lock the text for the duration of the job, as the original app did: the chunks
    // being generated refer to this exact text, so editing it mid-run would leave the
    // panel describing something the user can no longer see.
    isReadOnly = true;
    isTextCollapsed = true;
    isSettingsCollapsed = true;
    streamingJob = { id: ++streamingJobCounter, ...params };
  }

  function handleCloseStreaming() {
    streamingJob = null;
    // Unlike the original, which left the textarea read-only until a page reload, hand
    // editing back so the user can adjust the text and run again.
    isReadOnly = false;
    isTextCollapsed = false;
    isSettingsCollapsed = false;
    reloadedJob = null;
  }
</script>

<Toast />

<div class="background-elements">
  <div class="blob blob-1"></div>
  <div class="blob blob-2"></div>
</div>

<div class="app-container">
  <Header
    {currentUser}
    onOpenAuth={() => isAuthOpen = true}
    onOpenHistory={handleOpenMyHistory}
    onOpenAdmin={() => isAdminOpen = true}
    onLogout={handleLogout}
  />

  <main>
    <!-- Text Content Input Panel -->
    <TextInputPanel bind:text={mainText} bind:isReadOnly={isReadOnly} bind:isCollapsed={isTextCollapsed} inputPanelSpec={manifest?.ui_schema?.input_panel || null} {manifest} />

    <!-- Main Tabs dynamically ordered by Manifest -->
    <div id="main-tabs-container" style="margin-bottom: 1.5rem; width: 100%;">
      <div class="tabs">
        {#each activeModes as mode (mode.id)}
          <button
            class="tab-btn"
            class:active={activeTab === mode.id}
            onclick={() => activeTab = mode.id}
          >
            {mode.name}
          </button>
        {/each}
      </div>
    </div>

    <!--
      Universal Dynamic Audio Settings panel. Collapsible like the text panel, and
      collapsed automatically while a job runs so the streaming panel is what the user
      sees — the same accordion behaviour as the original app.
    -->
    <div class="glass-panel" class:collapsed={isSettingsCollapsed} style="margin-bottom: 1.5rem; transition: all 0.3s ease; {isSettingsCollapsed ? '' : 'min-height: 360px;'}">
      <div style="display: flex; justify-content: space-between; align-items: center;">
        <span style="font-size: 1.1rem; color: var(--primary); display: flex; align-items: center; gap: 8px; font-weight: 600;">
          <i class="fa-solid fa-sliders"></i> Audio Settings
        </span>
        <button
          onclick={() => isSettingsCollapsed = !isSettingsCollapsed}
          style="background: rgba(255,255,255,0.1); border: none; color: white; padding: 4px 10px; border-radius: 6px; font-size: 0.8em; display: flex; align-items: center; gap: 5px; cursor: pointer;"
        >
          <i class="fa-solid {isSettingsCollapsed ? 'fa-chevron-down' : 'fa-chevron-up'}"></i>
          <span>{isSettingsCollapsed ? 'Expand' : 'Collapse'}</span>
        </button>
      </div>

      {#if !isSettingsCollapsed}
        <div style="margin-top: 0.5rem;">
          {#if activeTab}
            <GenericEnginePanel
              text={mainText}
              activeMode={currentModeSpec}
              {manifest}
              {reloadedJob}
              onStartStreaming={handleStartStreaming}
            />
          {:else}
            <p style="color: var(--text-muted); font-size: 0.9rem; margin: 0;">
              Waiting for the engine to report its available modes…
            </p>
          {/if}
        </div>
      {/if}
    </div>

    <!--
      Streaming panel is a sibling of the settings panel, not a child of it, mirroring
      panel-settings-content vs streaming-panel in the original app. Keyed on the job so
      that starting a new job remounts it and re-splits the text — without the key it
      would keep the first job's chunks forever.
    -->
    {#if streamingJob}
      {#key streamingJob.id}
        <StreamingPanel
          text={mainText}
          engine={streamingJob.engine}
          voice={streamingJob.voice}
          speed={streamingJob.speed}
          pitch={streamingJob.pitch}
          emotion={streamingJob.emotion}
          {manifest}
          {reloadedJob}
          onClose={handleCloseStreaming}
        />
      {/key}
    {/if}
  </main>

  <footer>
    <p>AI Voice Studio &copy; 2026 - Powered by Svelte 5, Go Chi & Core AI Engine</p>
  </footer>
</div>

<AuthModal isOpen={isAuthOpen} onClose={() => isAuthOpen = false} onSuccess={handleAuthSuccess} />
<HistoryModal
  isOpen={isHistoryOpen}
  onClose={() => isHistoryOpen = false}
  onReloadJob={handleReloadJob}
  targetUserId={adminTargetUserId}
  targetUsername={adminTargetUsername}
/>
<AdminModal
  isOpen={isAdminOpen}
  onClose={() => isAdminOpen = false}
  onViewUserHistory={handleViewUserHistory}
/>
