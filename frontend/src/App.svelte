<script lang="ts">
  import { onMount } from 'svelte';
  import Header from './lib/components/Header.svelte';
  import TextInputPanel from './lib/components/TextInputPanel.svelte';
  import FastTtsTab from './lib/components/FastTtsTab.svelte';
  import StandardTtsTab from './lib/components/StandardTtsTab.svelte';
  import CloneTtsTab from './lib/components/CloneTtsTab.svelte';
  import HistoryModal from './lib/components/HistoryModal.svelte';
  import AuthModal from './lib/components/AuthModal.svelte';
  import AdminModal from './lib/components/AdminModal.svelte';
  import Toast from './lib/components/Toast.svelte';
  import { fetchVoices, fetchManifest, checkCurrentUser, logout, type UserResponse } from './lib/api';
  import { toast } from './lib/toast.svelte';
  import type { VoiceOption, UniversalManifest } from './lib/types';

  // Svelte 5 states using runes
  let activeTab = $state<'fasttts' | 'standard' | 'clone'>('fasttts');
  let mainText = $state(
    'Xin chào! Đây là ứng dụng tổng hợp giọng nói tiếng Việt VieNeu TTS Studio được xây dựng lại với Svelte 5. Chúc bạn có những trải nghiệm thật thú vị!'
  );
  let isReadOnly = $state(false);
  let reloadedJob = $state<any | null>(null);
  let voices = $state<VoiceOption[]>([]);
  let currentUser = $state<UserResponse | null>(null);
  let manifest = $state<UniversalManifest | null>(null);

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

    <!-- Main Tabs -->
    <div id="main-tabs-container" style="margin-bottom: 1.5rem; width: 100%;">
      <div class="tabs" style="margin-bottom: 0; width: 100%; display: flex; gap: 10px;">
        <button
          class="tab-btn"
          class:active={activeTab === 'fasttts'}
          onclick={() => selectTab('fasttts')}
          style="flex: 1;"
        >
          <i class="fa-solid fa-bolt"></i> Siêu Nhanh
        </button>
        <button
          class="tab-btn"
          class:active={activeTab === 'standard'}
          onclick={() => selectTab('standard')}
          style="flex: 1;"
        >
          <i class="fa-solid fa-wave-square"></i> TTS Cơ Bản
        </button>
        <button
          class="tab-btn"
          class:active={activeTab === 'clone'}
          onclick={() => selectTab('clone')}
          style="flex: 1;"
        >
          <i class="fa-solid fa-users-viewfinder"></i> Giọng Clone
        </button>
      </div>
    </div>

    <!-- Audio Settings & Generation Panel -->
    <div class="glass-panel" style="margin-bottom: 1.5rem;">
      {#if activeTab === 'fasttts'}
        <FastTtsTab text={mainText} {voices} {reloadedJob} modelOption={manifest?.ui_schema?.option_panel?.fast || null} />
      {:else if activeTab === 'standard'}
        <StandardTtsTab text={mainText} {voices} {reloadedJob} />
      {:else if activeTab === 'clone'}
        <CloneTtsTab text={mainText} {reloadedJob} />
      {/if}
    </div>
  </main>

  <footer>
    <p>VieNeu TTS Studio &copy; 2026 - Powered by Svelte 5 & FastAPI</p>
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
