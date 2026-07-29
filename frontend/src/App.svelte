<script lang="ts">
  import { onMount } from 'svelte';
  import Header from './lib/components/Header.svelte';
  import TextInputPanel from './lib/components/TextInputPanel.svelte';
  import GenericEnginePanel from './lib/components/GenericEnginePanel.svelte';
  import HistoryModal from './lib/components/HistoryModal.svelte';
  import AuthModal from './lib/components/AuthModal.svelte';
  import AdminModal from './lib/components/AdminModal.svelte';
  import Toast from './lib/components/Toast.svelte';
  import { fetchVoices, fetchManifest, checkCurrentUser, logout, type UserResponse } from './lib/api';
  import { toast } from './lib/toast.svelte';
  import type { VoiceOption, UniversalManifest } from './lib/types';

  // Svelte 5 states using runes
  let activeTab = $state<string>('fast');
  let mainText = $state(
    'Xin chào! Chào mừng bạn đến với AI Voice Studio. Chúc bạn có những trải nghiệm thật thú vị!'
  );
  let isReadOnly = $state(false);
  let reloadedJob = $state<any | null>(null);
  let voices = $state<VoiceOption[]>([]);
  let currentUser = $state<UserResponse | null>(null);
  let manifest = $state<UniversalManifest | null>(null);

  // Dynamic tab ordering derived from manifest ui_schema
  let activeModes = $derived.by(() => {
    if (!manifest) {
      return [
        { id: 'fast', name: 'Siêu Nhanh', icon: 'fa-bolt' },
        { id: 'standard', name: 'TTS Cơ Bản', icon: 'fa-wave-square' },
        { id: 'clone', name: 'Giọng Clone', icon: 'fa-users-viewfinder' }
      ];
    }
    const sortOrder = manifest.ui_schema?.model_sort;
    const allModes = manifest.supported_modes || [];
    if (sortOrder && sortOrder.length > 0) {
      return sortOrder.map((id) => {
        const modeSpec = allModes.find((m) => m.id === id);
        return {
          id,
          name: modeSpec ? modeSpec.name : id.toUpperCase(),
          icon: id === 'fast' ? 'fa-bolt' : id === 'standard' ? 'fa-wave-square' : id === 'clone' ? 'fa-users-viewfinder' : 'fa-sliders'
        };
      });
    }
    return allModes.map((m) => ({
      ...m,
      icon: m.id === 'fast' ? 'fa-bolt' : m.id === 'standard' ? 'fa-wave-square' : m.id === 'clone' ? 'fa-users-viewfinder' : 'fa-sliders'
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

  onMount(() => {
    // Check Auth status once on mount
    checkCurrentUser().then((user) => {
      currentUser = user;
      if (user) {
        loadData();
      } else {
        isAuthOpen = true;
      }
    });
  });

  async function loadData() {
    loadVoices();
    try {
      const m = await fetchManifest();
      if (m) {
        manifest = m;
      }
    } catch (e) {
      console.error('Failed to load manifest', e);
    }
  }

  function loadVoices() {
    fetchVoices().then((v) => {
      if (v && v.length > 0) {
        voices = v;
      }
    });
  }

  function handleAuthSuccess(user: UserResponse) {
    currentUser = user;
    isAuthOpen = false;
    loadData();
  }

  async function handleLogout() {
    await logout();
    currentUser = null;
    isAuthOpen = true;
    toast.show('Đã đăng xuất', 'info');
  }

  function selectTab(tab: 'fasttts' | 'standard' | 'clone') {
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

  function handleReloadJob(job: any) {
    mainText = job.text;
    isReadOnly = true;
    reloadedJob = job;
    if (job.engine === 'standard' || job.engine === 'fasttts' || job.engine === 'clone') {
      activeTab = job.engine;
    }
    isHistoryOpen = false;
    toast.show(`Đã nạp lại tác vụ ${job.job_id ? job.job_id.substring(0, 8) : ''}!`, 'success');
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
    <TextInputPanel bind:text={mainText} bind:isReadOnly={isReadOnly} inputPanelSpec={manifest?.ui_schema?.input_panel || null} />

    <!-- Main Tabs dynamically ordered by Manifest -->
    <div id="main-tabs-container" style="margin-bottom: 1.5rem; width: 100%;">
      <div class="tabs" style="margin-bottom: 0; width: 100%; display: flex; gap: 10px;">
        {#each activeModes as mode (mode.id)}
          <button
            class="tab-btn"
            class:active={activeTab === mode.id || (activeTab === 'fasttts' && mode.id === 'fast')}
            onclick={() => activeTab = mode.id}
            style="flex: 1;"
          >
            <i class="fa-solid {mode.icon}"></i> {mode.name}
          </button>
        {/each}
      </div>
    </div>

    <!-- Universal Dynamic Audio Settings & Generation Panel -->
    {#if manifest}
      <div class="glass-panel" style="margin-bottom: 1.5rem;">
        <GenericEnginePanel
          text={mainText}
          activeMode={currentModeSpec}
          {manifest}
          bind:voices={voices}
          {reloadedJob}
        />
      </div>
    {/if}
  </main>

  <footer>
    <p>AI Voice Studio &copy; 2026 - Powered by Svelte 5, Go Chi & Core AI Engine</p>
  </footer>
</div>

<AuthModal isOpen={isAuthOpen} onSuccess={handleAuthSuccess} />
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
