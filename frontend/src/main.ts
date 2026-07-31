import { hydrate, mount } from 'svelte'
import './app.css'
import App from './App.svelte'

declare global {
  interface Window {
    __SSG_MANIFEST__?: any;
  }
}

window.addEventListener('error', (e) => {
  console.error('[Global App Error]:', e.error || e.message);
});

const target = document.getElementById('app')!
const initialManifest = (window as any).__SSG_MANIFEST__ || null

// Clear pre-rendered HTML shell and cleanly mount Svelte 5 app to guarantee 100% event listeners
target.innerHTML = ''
const app = mount(App, { target, props: { initialManifest } })

export default app
