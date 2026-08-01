<script lang="ts">
  import { onMount } from 'svelte';
  import Header from './lib/components/Header.svelte';
  import TextInputPanel from './lib/components/TextInputPanel.svelte';
  import GenericEnginePanel from './lib/components/GenericEnginePanel.svelte';
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
  let activeTab = $state<string>('fast');
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
    <TextInputPanel bind:text={mainText} bind:isReadOnly={isReadOnly} inputPanelSpec={manifest?.ui_schema?.input_panel || null} {manifest} />

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

    <!-- Universal Dynamic Audio Settings & Generation Panel -->
    <div class="glass-panel" style="margin-bottom: 1.5rem; min-height: 360px;">
      <GenericEnginePanel
        text={mainText}
        activeMode={currentModeSpec}
        {manifest}
        {reloadedJob}
      />
    </div>
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
