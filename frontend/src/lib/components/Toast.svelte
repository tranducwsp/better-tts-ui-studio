<script lang="ts">
  import { toast } from '../toast.svelte';
</script>

<div id="toast-container">
  {#each toast.toasts as t (t.id)}
    <div class="toast {t.type}">
      <i class="fa-solid {t.type === 'success' ? 'fa-circle-check' : t.type === 'error' ? 'fa-circle-exclamation' : 'fa-info-circle'}"></i>
      <span>{t.message}</span>
    </div>
  {/each}
</div>
