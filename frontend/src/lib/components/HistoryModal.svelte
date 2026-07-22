<script lang="ts">
  import { fetchHistory } from '../api';
  import { toast } from '../toast.svelte';
  import type { HistoryItem } from '../types';

  interface Props {
    isOpen: boolean;
    onClose: () => void;
    onReloadJob: (job: any) => void;
    targetUserId?: string | null;
    targetUsername?: string | null;
  }

  let { isOpen, onClose, onReloadJob, targetUserId = null, targetUsername = null }: Props = $props();

  let history = $state<HistoryItem[]>([]);
  let isLoading = $state(false);

  $effect(() => {
    if (isOpen) {
      loadHistory();
    }
  });

  async function loadHistory() {
    isLoading = true;
    try {
      let data: HistoryItem[] = [];
      if (targetUserId) {
        const res = await fetch(`/api/admin/users/${targetUserId}/history`, { credentials: 'include' });
        if (!res.ok) throw new Error('Không thể tải lịch sử người dùng');
        data = await res.json();
      } else {
        data = await fetchHistory();
      }

      // Lọc bỏ các bản ghi trùng lặp nội dung & thời gian do vòng lặp cũ ghi rác vào DB
      const unique: HistoryItem[] = [];
      const seen = new Set<string>();

      for (const item of data) {
        const key = `${item.engine}_${item.voice}_${item.text}_${item.progress}_${item.time_ago}`;
        if (!seen.has(key)) {
          seen.add(key);
          unique.push(item);
        }
      }
      history = unique;
    } catch (e: any) {
      toast.show('Lỗi tải lịch sử: ' + e.message, 'error');
      history = [];
    } finally {
      isLoading = false;
    }
  }

  async function reloadJob(jobId: string) {
    try {
      const res = await fetch(`/api/history/${jobId}`, { credentials: 'include' });
      if (!res.ok) throw new Error('Không thể nạp lại tác vụ');
      const job = await res.json();
      onReloadJob(job);
    } catch (err: any) {
      toast.show('Lỗi nạp lại tác vụ: ' + err.message, 'error');
    }
  }
</script>

{#if isOpen}
  <div style="position: fixed; inset: 0; background: rgba(0,0,0,0.8); backdrop-filter: blur(8px); z-index: 10001; display: flex; align-items: center; justify-content: center; padding: 20px;">
    <div class="glass-panel modal-animate" style="width: 100%; max-width: 850px; max-height: 85vh; overflow-y: auto;">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; border-bottom: 1px solid var(--glass-border); padding-bottom: 15px;">
        <h2 style="color: var(--primary); display: flex; align-items: center; gap: 10px; font-size: 1.3rem;">
          <i class="fa-solid fa-clock-rotate-left"></i>
          {targetUsername ? `Lịch sử hoạt động của: ${targetUsername}` : 'Lịch sử tổng hợp âm thanh'}
        </h2>
        <button onclick={onClose} aria-label="Đóng lịch sử" style="background: none; border: none; color: white; font-size: 1.5rem; cursor: pointer;">
          <i class="fa-solid fa-xmark"></i>
        </button>
      </div>

      {#if isLoading}
        <p style="text-align: center; color: var(--text-muted); padding: 40px 0;">Đang tải lịch sử...</p>
      {:else if history.length === 0}
        <div style="text-align: center; padding: 40px 20px; color: #94a3b8;">
          <i class="fa-solid fa-box-open" style="font-size: 3rem; margin-bottom: 15px; opacity: 0.5;"></i>
          <p>Chưa có dữ liệu lịch sử nào.</p>
        </div>
      {:else}
        <table class="glass-table" style="font-size: 0.85rem; width: 100%;">
          <thead>
            <tr>
              <th style="padding: 10px; min-width: 100px;">Thời gian</th>
              <th style="padding: 10px; min-width: 130px;">Engine</th>
              <th style="padding: 10px; min-width: 130px;">Giọng đọc</th>
              <th style="padding: 10px; width: 35%;">Nội dung</th>
              <th style="padding: 10px; min-width: 80px;">Tiến trình</th>
              <th style="padding: 10px; min-width: 100px; text-align: center;">Hành động</th>
            </tr>
          </thead>
          <tbody>
            {#each history as item (item.job_id)}
              <tr>
                <td style="color: #a5b4fc; font-size: 0.8rem; font-weight: 500;">{item.time_ago}</td>
                <td style="min-width: 130px;">
                  {#if item.engine === 'standard'}
                    <span class="badge" style="background: rgba(59,130,246,0.2); color: #60a5fa; border: 1px solid rgba(59,130,246,0.3); padding: 4px 8px; display: inline-flex; align-items: center; gap: 4px;">
                      <i class="fa-solid fa-wave-square"></i> Standard
                    </span>
                  {:else if item.engine === 'fasttts'}
                    <span class="badge" style="background: rgba(16,185,129,0.2); color: #34d399; border: 1px solid rgba(16,185,129,0.3); padding: 4px 8px; display: inline-flex; align-items: center; gap: 4px;">
                      <i class="fa-solid fa-bolt"></i> FastTTS
                    </span>
                  {:else}
                    <span class="badge" style="background: rgba(236,72,153,0.2); color: #f472b6; border: 1px solid rgba(236,72,153,0.3); padding: 4px 8px; display: inline-flex; align-items: center; gap: 4px;">
                      <i class="fa-solid fa-users-viewfinder"></i> Clone
                    </span>
                  {/if}
                </td>
                <td>
                  <span style="font-weight: 600; color: #e2e8f0;">{item.voice}</span>
                  <span style="font-size: 0.8em; opacity: 0.8; color: #a855f7;">({item.speed}x)</span>
                </td>
                <td style="color: #cbd5e1; font-style: italic;">{item.text}</td>
                <td>
                  <span style="color: {item.is_complete ? '#10b981' : '#f59e0b'}; font-weight: 600;">
                    {item.progress}
                  </span>
                </td>
                <td style="text-align: center;">
                  <button
                    onclick={() => reloadJob(item.job_id)}
                    class="action-btn reload"
                    style="padding: 6px 12px; font-size: 0.8rem; background: rgba(99,102,241,0.2); border: 1px solid var(--primary); color: white; border-radius: 8px; cursor: pointer; display: inline-flex; align-items: center; gap: 5px; font-weight: 500;"
                    title="Nạp lại tác vụ này vào bảng điều khiển"
                  >
                    <i class="fa-solid fa-rotate-right"></i> Nạp lại
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  </div>
{/if}
