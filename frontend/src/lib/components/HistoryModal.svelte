<script lang="ts">
  import { fetchHistory, authFetch } from '../api';
  import type { HistoryCursor } from '../api';
  import { toast } from '../toast.svelte';
  import type { HistoryItem, JobDetailResponse } from '../types';

  interface Props {
    isOpen: boolean;
    onClose: () => void;
    onReloadJob: (job: JobDetailResponse) => void;
    targetUserId?: string | null;
    targetUsername?: string | null;
  }

  let { isOpen, onClose, onReloadJob, targetUserId = null, targetUsername = null }: Props = $props();

  let history = $state<HistoryItem[]>([]);
  let isLoading = $state(false);
  let hasMore = $state(false);
  let isLoadingMore = $state(false);

  $effect(() => {
    if (isOpen) {
      loadHistory();
    }
  });

  // requestHistoryPage gọi API lịch sử (người dùng của mình hoặc admin theo user_id) và trả về
  // cả mục lẫn cờ còn trang. Admin dùng cùng định dạng phân trang nên hai nhánh chia một code.
  async function requestHistoryPage(before: HistoryCursor | null): Promise<{ items: HistoryItem[]; has_more: boolean }> {
    if (targetUserId) {
      const params = new URLSearchParams();
      if (before) {
        params.set('before', before.created_at);
        params.set('before_id', before.job_id);
      }
      const qs = params.toString();
      const res = await authFetch(`/api/admin/users/${targetUserId}/history${qs ? `?${qs}` : ''}`);
      if (!res.ok) throw new Error('Failed to load user history');
      return await res.json();
    }
    return await fetchHistory(before);
  }

  async function loadHistory() {
    isLoading = true;
    try {
      const page = await requestHistoryPage(null);
      history = page.items;
      hasMore = page.has_more;
    } catch (e: unknown) {
      const errMsg = e instanceof Error ? e.message : String(e);
      toast.show('Failed to load history: ' + errMsg, 'error');
      history = [];
      hasMore = false;
    } finally {
      isLoading = false;
    }
  }

  // loadMore nối thêm trang sau vào danh sách đang hiển thị. Con trỏ là item cuối cùng hiện có —
  // keyset máy chủ dùng là (created_at, job_id). Cùng job có thể xuất hiện lại ở trang sau khi
  // hai job trùng timestamp, nên loại ngay khi id đã có ở trang trước.
  async function loadMore() {
    if (isLoadingMore || isLoading) return;
    const last = history[history.length - 1];
    if (!last) return;

    isLoadingMore = true;
    try {
      const page = await requestHistoryPage({ created_at: last.created_at, job_id: last.job_id });
      const seen = new Set(history.map((item) => item.job_id));
      const fresh = page.items.filter((item) => !seen.has(item.job_id));
      if (fresh.length > 0) {
        history = [...history, ...fresh];
      }
      hasMore = page.has_more;
    } catch (e: unknown) {
      const errMsg = e instanceof Error ? e.message : String(e);
      toast.show('Failed to load more history: ' + errMsg, 'error');
    } finally {
      isLoadingMore = false;
    }
  }

  async function reloadJob(jobId: string) {
    try {
      const res = await authFetch(`/api/history/${jobId}`);
      if (!res.ok) throw new Error('Failed to reload task');
      const job = await res.json();
      onReloadJob(job);
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.show('Failed to reload task: ' + errMsg, 'error');
    }
  }
</script>

{#if isOpen}
  <div style="position: fixed; inset: 0; background: rgba(0,0,0,0.8); backdrop-filter: blur(8px); z-index: 10001; display: flex; align-items: center; justify-content: center; padding: 20px;">
    <div class="glass-panel modal-animate" style="width: 100%; max-width: 850px; max-height: 85vh; overflow-y: auto;">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; border-bottom: 1px solid var(--glass-border); padding-bottom: 15px;">
        <h2 style="color: var(--primary); display: flex; align-items: center; gap: 10px; font-size: 1.3rem;">
          <i class="fa-solid fa-clock-rotate-left"></i>
          {targetUsername ? `Activity History of: ${targetUsername}` : 'Synthesis History'}
        </h2>
        <button onclick={onClose} aria-label="Close history modal" style="background: none; border: none; color: white; font-size: 1.5rem; cursor: pointer;">
          <i class="fa-solid fa-xmark"></i>
        </button>
      </div>

      {#if isLoading}
        <p style="text-align: center; color: var(--text-muted); padding: 40px 0;">Loading history...</p>
      {:else if history.length === 0}
        <div style="text-align: center; padding: 40px 20px; color: #94a3b8;">
          <i class="fa-solid fa-box-open" style="font-size: 3rem; margin-bottom: 15px; opacity: 0.5;"></i>
          <p>No history records found.</p>
        </div>
      {:else}
        <div class="glass-table-wrapper" style="overflow-x: auto; width: 100%; -webkit-overflow-scrolling: touch;">
          <table class="glass-table" style="font-size: 0.85rem; width: 100%;">
          <thead>
            <tr>
              <th style="padding: 10px; min-width: 100px;">Time</th>
              <th style="padding: 10px; min-width: 130px;">Engine</th>
              <th style="padding: 10px; min-width: 130px;">Voice</th>
              <th style="padding: 10px; width: 35%;">Content</th>
              <th style="padding: 10px; min-width: 80px;">Progress</th>
              <th style="padding: 10px; min-width: 100px; text-align: center;">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each history as item (item.job_id)}
              <tr>
                <td style="color: #a5b4fc; font-size: 0.8rem; font-weight: 500;">{item.time_ago}</td>
                <td style="min-width: 130px;">
                  <span class="badge" style="background: rgba(99,102,241,0.2); color: #a5b4fc; border: 1px solid rgba(99,102,241,0.3); padding: 4px 8px; display: inline-flex; align-items: center; gap: 4px; text-transform: capitalize;">
                    <i class="fa-solid fa-microchip"></i> {item.engine}
                  </span>
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
                    title="Reload this task into input panel"
                  >
                    <i class="fa-solid fa-rotate-right"></i> Reload
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
        {#if hasMore}
          <div style="display: flex; justify-content: center; padding: 16px 0 8px;">
            <button
              onclick={loadMore}
              disabled={isLoadingMore}
              style="padding: 8px 20px; font-size: 0.85rem; background: rgba(99,102,241,0.15); border: 1px solid rgba(99,102,241,0.4); color: white; border-radius: 8px; cursor: pointer; display: inline-flex; align-items: center; gap: 6px; font-weight: 500;"
            >
              {#if isLoadingMore}
                <i class="fa-solid fa-spinner fa-spin"></i> Loading...
              {:else}
                <i class="fa-solid fa-angles-down"></i> Load more
              {/if}
            </button>
          </div>
        {/if}
      {/if}
    </div>
  </div>
{/if}
