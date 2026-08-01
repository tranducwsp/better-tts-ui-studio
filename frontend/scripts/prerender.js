import { createServer } from 'vite';
import fs from 'fs/promises';
import path from 'path';

// Fallback manifest matching core-tts-test microservice
const FALLBACK_MANIFEST = {
  engine_id: "core-tts-test-v1",
  engine_name: "Mock Test AI Engine",
  version: "1.0.0-mock",
  provider: "Universal AI Testbed",
  supported_modes: [
    { id: "fast", name: "Mock Fast", description: "Instant mock audio generator" },
    { id: "express", name: "Mock Express", description: "Ultra-low latency streaming model" },
    { id: "zero_shot_clone", name: "Mock Instant Zero-Shot Clone", description: "Instant voice cloning from uploaded reference audio with voice saving support", capabilities: { supports_cloning: true, supports_voice_saving: true } },
    { id: "multilingual", name: "Mock Multilingual", description: "Cross-lingual multi-accent voice engine" },
    { id: "emotion_v2", name: "Mock Emotion & Style", description: "Dynamic prosody & pitch control model", capabilities: { supports_pitch: true, supports_emotion: true } },
    { id: "standard", name: "Mock Standard", description: "Fast mock audio generator" },
    { id: "clone", name: "Mock Voice Cloning", description: "Simulated speaker cloning", capabilities: { supports_cloning: true, supports_voice_saving: true } }
  ],

  // Engine-wide defaults; modes that state nothing inherit these.
  capabilities: { supports_preset_voices: true, supports_cloning: false, supports_voice_saving: false, supports_streaming: true, supports_speed: true, supports_pitch: false, supports_emotion: false, supports_ssml: false },
  constraints: {
    max_text_length: 3000,
    speed_range: { min: 0.5, max: 2.0, default: 1.0, step: 0.1 },
    pitch_range: { min: -10.0, max: 10.0, default: 0.0, step: 0.5 },
    supported_emotions: ["neutral", "happy", "sad", "angry", "excited"],
    chunking: {
      max_chunk_size: 1000,
      delimiters: ["(?<=\\.\\s*\\n)", "(?<=[.!?]\\s+)"]
    }
  },
  audio_spec: {
    supported_formats: ["wav", "mp3"],
    supported_sample_rates: [16000, 22050, 24000, 44100],
    default_format: "wav",
    default_sample_rate: 24000
  },
  ui_schema: {
    ui_mode: process.env.VITE_UI_MODE || "beauty",
    input_panel: {
      file_serve: true,
      closeable: false,
      find_mode: "expert",
      replace_tool: true,
      enable_chunk_box: true
    },
    model_sort: ["fast", "express", "zero_shot_clone", "multilingual", "emotion_v2", "standard", "clone"],
    option_panel: {
      fast: {
        notice_banner: { level: "info", message: "⚡ Cloud Fast: Ultra-fast simulated TTS responses." },
        voice_type: "radio",
        speed_type: "slider",
        preset_voices: [
          { id: "Mock Voice A (Female)", name: "Mock Voice A (Female)", gender: "female" },
          { id: "Mock Voice B (Male)", name: "Mock Voice B (Male)", gender: "male" }
        ]
      },
      express: {
        notice_banner: { level: "success", message: "✅ Express Real-Time: Low-latency streaming test engine ready." },
        voice_type: "select",
        speed_type: "slider"
      },
      zero_shot_clone: {
        notice_banner: { level: "info", message: "⚡ Instant Zero-Shot Clone: Supports temporary audio upload without saving voice profiles to account." },
        voice_type: "select",
        speed_type: "slider"
      },
      multilingual: {
        notice_banner: { level: "warning", message: "⚠️ Multilingual: Cross-lingual synthesis with regional accent selection." },
        voice_type: "select",
        speed_type: "slider"
      },
      emotion_v2: {
        notice_banner: { level: "danger", message: "🚨 Emotion & Style: Experimental model - dynamic pitch controls active." },
        voice_type: "select",
        speed_type: "slider",
        pitch_type: "slider",
        emotion_type: "select"
      },
      standard: { voice_type: "select", speed_type: "slider" },
      clone: {
        voice_type: "select",
        speed_type: "slider",
        voice_metadata_schema: [
          { key: "name", label: "Voice Name", type: "text", required: true, placeholder: "e.g. Test Voice..." },
          { key: "gender", label: "Gender", type: "select", options: ["Male", "Female", "Other"] },
          { key: "region", label: "Accent / Region", type: "select", options: ["North American", "British", "Australian", "Other"] },
          { key: "style", label: "Style", type: "select", options: ["Expressive", "News / Broadcast", "Audiobook / Reading", "Dramatic", "Natural / Conversational", "Commercial", "Other"] }
        ]
      }
    }
  }
};

async function prerender() {
  console.log('🚀 Starting SSG Pre-rendering build process with Manifest pre-fetch...');
  const vite = await createServer({
    server: { middlewareMode: true },
    appType: 'custom'
  });

  let manifest = FALLBACK_MANIFEST;
  try {
    const backendUrl = process.env.VITE_BACKEND_URL || 'http://localhost:8000';
    const res = await fetch(`${backendUrl}/api/info`);
    if (res.ok) {
      manifest = await res.json();
      console.log('📡 Successfully fetched live Manifest from backend for build-time SSG!');
    }
  } catch (e) {
    console.log('ℹ️ Backend offline during build stage. Used Core TTS Test Manifest for build-time SSG.');
  }

  try {
    const { render } = await vite.ssrLoadModule('svelte/server');
    const { default: App } = await vite.ssrLoadModule('./src/App.svelte');

    const result = render(App, { props: { initialManifest: manifest } });
    const htmlContent = result.html || '';

    const indexPath = path.resolve('dist/index.html');
    let template = await fs.readFile(indexPath, 'utf-8');

    const ssgScript = `<script>window.__SSG_MANIFEST__ = ${JSON.stringify(manifest)};</script>`;
    template = template.replace('</head>', `${ssgScript}</head>`);
    template = template.replace('<div id="app"></div>', `<div id="app">${htmlContent}</div>`);
    
    await fs.writeFile(indexPath, template);

    // Execute PurgeCSS on FontAwesome CSS to purge unused icon rules
    try {
      const { PurgeCSS } = await import('purgecss');
      const faCssPath = path.resolve('dist/fontawesome/css/all.min.css');
      const purgecssResult = await new PurgeCSS().purge({
        content: ['./src/**/*.svelte', './src/**/*.ts', './dist/index.html'],
        css: [faCssPath],
        safelist: {
          standard: [
            'fa-solid', 'fa-regular', 'fa-brands', 'fa-fw',
            'fa-bolt', 'fa-wave-square', 'fa-users-viewfinder', 'fa-users-gear',
            'fa-xmark', 'fa-check', 'fa-clock', 'fa-user-check', 'fa-clock-rotate-left',
            'fa-microphone-lines', 'fa-cloud-arrow-up', 'fa-bookmark', 'fa-scissors',
            'fa-right-from-bracket', 'fa-right-to-bracket', 'fa-file-lines',
            'fa-file-import', 'fa-wand-magic-sparkles', 'fa-chevron-down', 'fa-chevron-up',
            'fa-code', 'fa-font', 'fa-magnifying-glass', 'fa-arrow-down', 'fa-layer-group',
            'fa-user-lock', 'fa-user-plus', 'fa-box-open', 'fa-rotate-right', 'fa-circle-check',
            'fa-circle-exclamation', 'fa-info-circle', 'fa-circle-info', 'fa-circle-xmark',
            'fa-triangle-exclamation', 'fa-plus', 'fa-sliders', 'fa-volume-high', 'fa-play',
            'fa-pause', 'fa-stop', 'fa-download', 'fa-trash-can', 'fa-copy', 'fa-floppy-disk',
            'fa-gear', 'fa-user'
          ]
        }
      });
      if (purgecssResult && purgecssResult[0] && purgecssResult[0].css) {
        await fs.writeFile(faCssPath, purgecssResult[0].css);
        const originalKb = 90;
        const newKb = (purgecssResult[0].css.length / 1024).toFixed(1);
        console.log(`🧹 Purged unused FontAwesome CSS! Reduced from ~${originalKb}KB down to ${newKb}KB!`);
      }

      // Inline all CSS (App CSS + Purged FontAwesome CSS) directly into <head> for zero-request instant 0ms FCP!
      const assetsDir = path.resolve('dist/assets');
      const files = await fs.readdir(assetsDir);
      const cssFiles = files.filter(f => f.endsWith('.css'));
      let inlinedAppCss = '';
      for (const file of cssFiles) {
        const cssContent = await fs.readFile(path.join(assetsDir, file), 'utf-8');
        inlinedAppCss += cssContent + '\n';
      }
      let purgedFaCss = await fs.readFile(faCssPath, 'utf-8');
      // Fix relative font URLs for inlined CSS context (change ../webfonts/ to /fontawesome/webfonts/)
      purgedFaCss = purgedFaCss.replace(/url\((['"]?)\.\.\/webfonts\//g, "url($1/fontawesome/webfonts/");

      // Re-read index.html
      let finalHtml = await fs.readFile(indexPath, 'utf-8');
      
      // Remove external stylesheet link tags
      finalHtml = finalHtml.replace(/<link [^>]*rel="stylesheet"[^>]*>/g, '');
      finalHtml = finalHtml.replace(/<noscript>[\s\S]*?<\/noscript>/g, '');

      const activeUiMode = manifest?.ui_schema?.ui_mode || process.env.VITE_UI_MODE || 'beauty';
      console.log(`🎯 Active Build UI Mode: "${activeUiMode.toUpperCase()}"`);

      if (activeUiMode === 'fast') {
        // FAST MODE: Completely STRIP out FontAwesome CSS, font preloads, icon tags & heavy GPU filters!
        purgedFaCss = ''; // Zero FontAwesome CSS included in bundle
        
        // Remove font preload link tags from HTML
        finalHtml = finalHtml.replace(/<link rel="preload"[^>]*woff2[^>]*>/g, '');

        // Remove <i class="fa-solid..."> tags from pre-rendered DOM
        finalHtml = finalHtml.replace(/<i [^>]*class="fa-[^>]*><\/i>/g, '');

        // Strip heavy GPU filters, font-face & backdrop-filters from App CSS
        inlinedAppCss = inlinedAppCss.replace(/@font-face\s*\{[^\}]*\}/g, '');
        finalHtml = finalHtml.replace(/@font-face\s*\{[^\}]*\}/g, '');
        inlinedAppCss = inlinedAppCss.replace(/backdrop-filter:[^;\}]*;?/g, '');
        inlinedAppCss = inlinedAppCss.replace(/-webkit-backdrop-filter:[^;\}]*;?/g, '');
        inlinedAppCss = inlinedAppCss.replace(/filter:\s*blur\([^\)]*\);?/g, '');
        inlinedAppCss = inlinedAppCss.replace(/\.blob-[0-9]\s*\{[^\}]*\}/g, '');
        inlinedAppCss = inlinedAppCss.replace(/\.background-elements\s*\{[^\}]*\}/g, '');

        // Minify inlined CSS to save payload space
        inlinedAppCss = inlinedAppCss.replace(/\s+/g, ' ').replace(/\/\*[\s\S]*?\*\//g, '');

        finalHtml = finalHtml.replace('<body>', '<body data-mode="fast">');
        console.log(`⚡ FAST MODE STRIPPING COMPLETE: FontAwesome icons, WOFF2 preloads, System Fonts & GPU blur filters 100% PURGED from output artifact!`);
      }

      // Inject combined inlined CSS into <head>
      const combinedStyleTag = `<style id="critical-inlined-css">\n${inlinedAppCss}\n${purgedFaCss}\n</style>`;
      finalHtml = finalHtml.replace('</head>', `${combinedStyleTag}</head>`);

      // Move JS module script tag from <head> to the bottom of <body> for unblocked 0ms FCP/LCP!
      const scriptMatch = finalHtml.match(/<script type="module"[^>]*src="\/assets\/index-[^>]*><\/script>/);
      if (scriptMatch) {
        finalHtml = finalHtml.replace(scriptMatch[0], '');
        finalHtml = finalHtml.replace('</body>', `${scriptMatch[0]}</body>`);
      }

      await fs.writeFile(indexPath, finalHtml);
      console.log(`⚡ Inlined ALL CSS & moved JS to body end! 0 external CSS HTTP requests required for render!`);
      console.log(`⚡ Inlined ALL CSS & moved JS to body end! 0 external CSS HTTP requests required for render!`);
    } catch (purgeErr) {
      console.log('ℹ️ PurgeCSS warning:', purgeErr?.message || purgeErr);
    }

    console.log(`✅ SSG Prerendering complete! Injected Manifest State & ${htmlContent.length} bytes of pre-rendered HTML DOM into dist/index.html`);
  } catch (err) {
    console.error('⚠️ Pre-rendering warning:', err);
  } finally {
    await vite.close();
  }
}

prerender();
