import { hydrate, mount } from 'svelte'
import './app.css'
import App from './App.svelte'

declare global {
  interface Window {
    __SSG_MANIFEST__?: any;
  }
}

const target = document.getElementById('app')!
const initialManifest = window.__SSG_MANIFEST__ || null

// Standardized Single-Path Hydration Contract
const app = target.children.length > 0
  ? hydrate(App, { target, props: { initialManifest } })
  : mount(App, { target, props: { initialManifest } })

export default app
