<script lang="ts">
  import type { UserResponse } from '../api';

  interface Props {
    currentUser: UserResponse | null;
    onOpenAuth: () => void;
    onOpenHistory: () => void;
    onOpenAdmin: () => void;
    onLogout: () => void;
  }

  let { currentUser, onOpenAuth, onOpenHistory, onOpenAdmin, onLogout }: Props = $props();
</script>

<header style="margin-bottom: 1rem;">
  <div id="header-actions" style="display: flex; justify-content: space-between; align-items: center; z-index: 9997;">
    {#if currentUser}
      <div style="display: flex; gap: 12px; align-items: center;">
        <div class="user-badge" title="Tài khoản đang hoạt động">
          <span class="status-dot"></span>
          <span class="user-name">{currentUser.username}</span>
        </div>

        <button onclick={onOpenHistory} class="header-btn">
          <i class="fa-solid fa-clock-rotate-left"></i> Lịch sử
        </button>
      </div>

      <div style="display: flex; gap: 10px; align-items: center; margin-left: auto;">
        {#if currentUser.role === 'admin'}
          <button onclick={onOpenAdmin} class="header-btn danger-btn">
            <i class="fa-solid fa-users-gear"></i> Quản trị
          </button>
        {/if}

        <button onclick={onLogout} class="header-btn">
          <i class="fa-solid fa-right-from-bracket"></i> Đăng xuất
        </button>
      </div>
    {:else}
      <div style="margin-left: auto;">
        <button onclick={onOpenAuth} class="header-btn primary-btn" style="padding: 6px 16px;">
          <i class="fa-solid fa-right-to-bracket"></i> Đăng nhập / Đăng ký
        </button>
      </div>
    {/if}
  </div>
</header>

<style>
  .user-badge {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    background: rgba(255, 255, 255, 0.06);
    border: 1px solid rgba(255, 255, 255, 0.12);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    padding: 6px 14px;
    border-radius: 20px;
    font-size: 0.88rem;
    font-weight: 600;
    color: #f1f5f9;
    letter-spacing: 0.3px;
    box-shadow: 0 4px 15px rgba(0, 0, 0, 0.2);
    transition: all 0.25s ease;
    user-select: none;
  }

  .user-badge:hover {
    background: rgba(99, 102, 241, 0.15);
    border-color: rgba(99, 102, 241, 0.4);
    box-shadow: 0 0 12px rgba(99, 102, 241, 0.3);
  }

  .status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: #10b981;
    box-shadow: 0 0 8px #10b981;
  }
</style>
