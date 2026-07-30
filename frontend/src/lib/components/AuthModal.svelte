<script lang="ts">
  import { login, register, type UserResponse } from '../api';
  import { toast } from '../toast.svelte';

  interface Props {
    isOpen: boolean;
    onSuccess: (user: UserResponse) => void;
  }

  let { isOpen, onSuccess }: Props = $props();

  let mode = $state<'login' | 'register'>('login');
  let username = $state('');
  let password = $state('');
  let isLoading = $state(false);

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    if (!username.trim() || !password.trim()) {
      toast.show('Please enter both Username & Password', 'error');
      return;
    }

    isLoading = true;
    try {
      if (mode === 'login') {
        const user = await login(username, password);
        toast.show('Logged in successfully!', 'success');
        onSuccess(user);
      } else {
        await register(username, password);
        toast.show('Registration successful! Please wait for Admin approval.', 'success');
        mode = 'login';
      }
    } catch (err: any) {
      toast.show(err.message || 'Operation failed', 'error');
    } finally {
      isLoading = false;
    }
  }
</script>

{#if isOpen}
  <div style="position: fixed; inset: 0; background: rgba(0,0,0,0.85); backdrop-filter: blur(12px); z-index: 10000; display: flex; align-items: center; justify-content: center; padding: 20px;">
    <div class="glass-panel modal-animate" style="width: 100%; max-width: 420px; padding: 2rem; border-radius: 20px;">
      <div style="text-align: center; margin-bottom: 1.5rem;">
        <i class="fa-solid fa-user-lock" style="font-size: 2.5rem; color: var(--primary); margin-bottom: 10px;"></i>
        <h2 style="font-size: 1.5rem; font-weight: 700; color: white;">
          {mode === 'login' ? 'Studio Sign In' : 'Account Registration'}
        </h2>
        <p style="font-size: 0.85rem; color: var(--text-muted); margin-top: 5px;">
          {mode === 'login' ? 'Please log in to use voice synthesis features' : 'Create a new account to access the service'}
        </p>
      </div>

      <form onsubmit={handleSubmit} style="display: flex; flex-direction: column; gap: 1rem;">
        <div>
          <label for="auth-username" style="font-size: 0.85rem; color: #94a3b8; margin-bottom: 4px;">Username</label>
          <input
            id="auth-username"
            type="text"
            bind:value={username}
            placeholder="Enter username..."
            required
            style="width: 100%; padding: 12px; border-radius: 8px; background: rgba(15,23,42,0.8); border: 1px solid var(--glass-border); color: white;"
          />
        </div>

        <div>
          <label for="auth-password" style="font-size: 0.85rem; color: #94a3b8; margin-bottom: 4px;">Password</label>
          <input
            id="auth-password"
            type="password"
            bind:value={password}
            placeholder="Enter password..."
            required
            style="width: 100%; padding: 12px; border-radius: 8px; background: rgba(15,23,42,0.8); border: 1px solid var(--glass-border); color: white;"
          />
        </div>

        <button type="submit" disabled={isLoading} class="btn primary-btn" style="margin-top: 10px; padding: 12px;">
          <i class="fa-solid {mode === 'login' ? 'fa-right-to-bracket' : 'fa-user-plus'}"></i>
          {isLoading ? 'Processing...' : (mode === 'login' ? 'Sign In' : 'Register')}
        </button>
      </form>

      <div style="margin-top: 1.5rem; text-align: center; font-size: 0.9rem; border-top: 1px solid var(--glass-border); padding-top: 1rem;">
        {#if mode === 'login'}
          <span style="color: var(--text-muted);">Don't have an account?</span>
          <button type="button" onclick={() => mode = 'register'} style="background: none; border: none; color: var(--primary); font-weight: 600; cursor: pointer; margin-left: 5px;">
            Register now
          </button>
        {:else}
          <span style="color: var(--text-muted);">Already have an account?</span>
          <button type="button" onclick={() => mode = 'login'} style="background: none; border: none; color: var(--primary); font-weight: 600; cursor: pointer; margin-left: 5px;">
            Sign In
          </button>
        {/if}
      </div>
    </div>
  </div>
{/if}
