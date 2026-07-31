<script lang="ts">
  import { fetchAdminUsers, approveUser, type UserResponse } from '../api';
  import { toast } from '../toast.svelte';

  interface Props {
    isOpen: boolean;
    onClose: () => void;
    onViewUserHistory: (userId: string, username: string) => void;
  }

  let { isOpen, onClose, onViewUserHistory }: Props = $props();

  let users = $state<UserResponse[]>([]);
  let isLoading = $state(false);

  $effect(() => {
    if (isOpen) {
      loadUsers();
    }
  });

  async function loadUsers() {
    isLoading = true;
    try {
      users = await fetchAdminUsers();
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.show('Failed to load user list: ' + errMsg, 'error');
    } finally {
      isLoading = false;
    }
  }

  async function handleApprove(userId: string) {
    try {
      await approveUser(userId);
      toast.show('User approved successfully!', 'success');
      loadUsers();
    } catch (err: unknown) {
      const errMsg = err instanceof Error ? err.message : String(err);
      toast.show('Approval error: ' + errMsg, 'error');
    }
  }
</script>

{#if isOpen}
  <div style="position: fixed; inset: 0; background: rgba(0,0,0,0.8); backdrop-filter: blur(10px); z-index: 9999; display: flex; align-items: center; justify-content: center; padding: 20px;">
    <div class="glass-panel modal-animate" style="width: 100%; max-width: 800px; max-height: 85vh; overflow-y: auto;">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; border-bottom: 1px solid var(--glass-border); padding-bottom: 15px;">
        <h2 style="color: var(--danger); display: flex; align-items: center; gap: 10px; font-size: 1.3rem;">
          <i class="fa-solid fa-users-gear"></i> User Management & Account Approval
        </h2>
        <button onclick={onClose} aria-label="Close admin panel" style="background: none; border: none; color: white; font-size: 1.5rem; cursor: pointer;">
          <i class="fa-solid fa-xmark"></i>
        </button>
      </div>

      {#if isLoading}
        <p style="text-align: center; color: var(--text-muted); padding: 30px 0;">Loading user list...</p>
      {:else if users.length === 0}
        <p style="text-align: center; color: var(--text-muted); padding: 30px 0;">No users found in system.</p>
      {:else}
        <table class="glass-table" style="width: 100%;">
          <thead>
            <tr>
              <th>ID</th>
              <th>Username</th>
              <th>Role</th>
              <th>Status</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each users as u (u.id)}
              <tr>
                <td style="font-family: monospace; font-size: 0.8rem;">{u.id.substring(0, 8)}...</td>
                <td><strong>{u.username}</strong></td>
                <td><span class="badge" style="background: rgba(255,255,255,0.1);">{u.role}</span></td>
                <td>
                  {#if u.is_approved}
                    <span class="badge badge-success"><i class="fa-solid fa-check"></i> Approved</span>
                  {:else}
                    <span class="badge badge-warning"><i class="fa-solid fa-clock"></i> Pending</span>
                  {/if}
                </td>
                <td style="display: flex; gap: 8px;">
                  {#if !u.is_approved}
                    <button onclick={() => handleApprove(u.id)} class="action-btn approve" style="padding: 4px 10px; font-size: 0.8rem; background: #10b981; color: white; border: none; border-radius: 6px; cursor: pointer;">
                      <i class="fa-solid fa-user-check"></i> Approve
                    </button>
                  {/if}
                  <button onclick={() => onViewUserHistory(u.id, u.username)} class="action-btn history" style="padding: 4px 10px; font-size: 0.8rem; background: rgba(99,102,241,0.2); border: 1px solid var(--primary); color: white; border-radius: 6px; cursor: pointer;">
                    <i class="fa-solid fa-clock-rotate-left"></i> History
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
