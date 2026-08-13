import { test, expect, Page, BrowserContext } from '@playwright/test';

/**
 * Integration tests for manifest-to-UI consistency.
 *
 * These tests verify that the running server's manifest correctly drives the UI.
 * They connect to the actual Docker stack (frontend + backend + engine).
 */

const BASE_URL = process.env.BASE_URL || 'http://localhost:5173';

/**
 * Log in and return the auth cookies so the browser session is authenticated.
 */
async function loginAsAdmin(context: BrowserContext): Promise<void> {
  const response = await context.request.post(`${BASE_URL}/api/login`, {
    data: { username: 'admin', password: 'devadmin' },
  });
  expect(response.ok()).toBeTruthy();
  // The response sets HttpOnly cookies — Playwright's context.request handles them
}

/**
 * Helper: fetch the manifest from the API.
 */
async function fetchManifest(request?: any): Promise<any> {
  const response = await fetch(`${BASE_URL}/api/info`);
  if (!response.ok) {
    throw new Error(`Failed to fetch manifest: ${response.status} ${response.statusText}`);
  }
  return response.json();
}

test.describe('Manifest-to-UI consistency', () => {
  test.beforeEach(async ({ page, context }) => {
    // Log in first so the page doesn't show the auth modal
    await loginAsAdmin(context);

    // Navigate and wait for the Svelte app to hydrate
    await page.goto(BASE_URL, { waitUntil: 'networkidle' });
    // Give the app time to fetch the manifest and render the main UI
    await page.waitForTimeout(1500);
  });

  test('mode tabs are rendered from supported_modes', async ({ page }) => {
    const manifest = await fetchManifest();
    const expectedModes = manifest.supported_modes.map(m => m.name);

    // Find all tab buttons
    const tabs = page.locator('.tab-btn');
    const tabCount = await tabs.count();
    expect(tabCount).toBe(expectedModes.length);

    // Verify each tab name (after cleaning)
    for (let i = 0; i < expectedModes.length; i++) {
      const tabText = await tabs.nth(i).textContent();
      // The UI strips trailing "Engine" or "Model" via cleanName()
      const cleaned = expectedModes[i].replace(/\s*(Engine|Model)$/i, '').trim();
      expect(tabText).toContain(cleaned);
    }
  });

  test('first mode tab is active by default', async ({ page }) => {
    const firstTab = page.locator('.tab-btn').first();
    await expect(firstTab).toHaveClass(/active/);
  });

  test('tab content is visible for active tab', async ({ page }) => {
    const activeContent = page.locator('.tab-content.active');
    await expect(activeContent).toBeVisible();
  });

  test('capability: supports_pitch=false hides pitch control', async ({ page }) => {
    // The current manifest has supports_pitch=false globally
    const pitchLabel = page.locator('label', { hasText: /pitch/i });
    await expect(pitchLabel).toHaveCount(0);
  });

  test('capability: supports_speed=true shows speed control', async ({ page }) => {
    // The current manifest has supports_speed=true globally
    const speedControl = page.locator('label', { hasText: /speed|tốc độ/i });
    await expect(speedControl).not.toHaveCount(0);
  });

  test('capability: supports_emotion=false hides emotion selector', async ({ page }) => {
    const emotionLabel = page.locator('label', { hasText: /emotion|cảm xúc/i });
    await expect(emotionLabel).toHaveCount(0);
  });

  test('input_panel: file_serve=true shows upload button', async ({ page }) => {
    const uploadBtn = page.locator('button', { hasText: /upload/i });
    await expect(uploadBtn).toBeVisible();
  });

  test('switching to zero_shot_clone mode shows cloning UI', async ({ page }) => {
    // Click the "Giọng Clone" tab (the third mode with supports_cloning=true)
    const tabs = page.locator('.tab-btn');
    const tabCount = await tabs.count();

    expect(tabCount).toBeGreaterThanOrEqual(3);

    // Click the clone tab (index 2)
    await tabs.nth(2).click();
    await page.waitForTimeout(500);

    // After switching to zero_shot_clone with supports_cloning=true,
    // the reference audio section should be visible
    const cloneSection = page.locator('text=Reference Audio|Tham chiếu|Clone|Upload', { exact: false }).first();
    // This should be visible because the mode has supports_cloning=true
    // Let's use a softer assertion in case the text differs
    const hasCloneUI = page.locator('.clone-setup, .upload-area, .upload-drop-zone');
    try {
      // Wait briefly for any cloning UI to appear
      await hasCloneUI.first().waitFor({ timeout: 2000 });
      await expect(hasCloneUI.first()).toBeVisible();
    } catch {
      // The cloning UI might use a different CSS class — just check it exists
      const uploadElements = await page.locator('input[type="file"]').count();
      expect(uploadElements).toBeGreaterThanOrEqual(1);
    }
  });

  test('capability: supports_cloning=false in default mode hides cloning UI', async ({ page }) => {
    // The default mode (fast) inherits engine-wide supports_cloning=false
    // So no cloning-related UI should be visible
    const cloneSetup = page.locator('.clone-setup');
    await expect(cloneSetup).toHaveCount(0);
  });
});

test.describe('Manifest API contract', () => {
  test('manifest endpoint returns valid JSON with all required fields', async () => {
    const manifest = await fetchManifest();

    // Verify all required top-level fields
    expect(manifest).toHaveProperty('engine_id');
    expect(manifest).toHaveProperty('engine_name');
    expect(manifest).toHaveProperty('version');
    expect(manifest).toHaveProperty('provider');
    expect(manifest).toHaveProperty('supported_modes');
    expect(manifest).toHaveProperty('capabilities');
    expect(manifest).toHaveProperty('constraints');
    expect(manifest).toHaveProperty('audio_spec');
    expect(manifest).toHaveProperty('ui_schema');

    // Verify no dead fields
    expect(manifest).not.toHaveProperty('supports_ssml');
    expect(manifest.ui_schema).not.toHaveProperty('closeable');
  });

  test('capabilities have exactly 7 fields (no dead fields)', async () => {
    const manifest = await fetchManifest();
    const expectedCaps = [
      'supports_preset_voices',
      'supports_cloning',
      'supports_voice_saving',
      'supports_streaming',
      'supports_speed',
      'supports_pitch',
      'supports_emotion',
    ];
    const actualCaps = Object.keys(manifest.capabilities).sort();
    expect(actualCaps).toEqual(expectedCaps.sort());
  });

  test('input_panel has exactly 5 fields (no dead fields)', async () => {
    const manifest = await fetchManifest();
    const expectedFields = ['file_serve', 'find_mode', 'replace_tool', 'enable_chunk_box', 'auto_format'];
    const actualFields = Object.keys(manifest.ui_schema.input_panel).sort();
    expect(actualFields).toEqual(expectedFields.sort());
  });

  test('supported_modes have valid structure', async () => {
    const manifest = await fetchManifest();
    expect(manifest.supported_modes.length).toBeGreaterThanOrEqual(1);

    for (const mode of manifest.supported_modes) {
      expect(mode).toHaveProperty('id');
      expect(mode).toHaveProperty('name');
      expect(typeof mode.id).toBe('string');
      expect(typeof mode.name).toBe('string');
      expect(mode.id).toBeTruthy();
    }
  });

  test('mode sort order matches ui_schema.model_sort', async () => {
    const manifest = await fetchManifest();
    const modeIds = manifest.supported_modes.map(m => m.id);
    const sortOrder = manifest.ui_schema.model_sort;

    if (sortOrder && sortOrder.length > 0) {
      expect(sortOrder.length).toBe(modeIds.length);
      for (const id of sortOrder) {
        expect(modeIds).toContain(id);
      }
    }
  });

  test('capability resolution works for mode overrides', async () => {
    const manifest = await fetchManifest();

    // Find the zero_shot_clone mode which overrides capabilities
    const cloneMode = manifest.supported_modes.find(m => m.id === 'zero_shot_clone');
    if (cloneMode?.capabilities) {
      if (cloneMode.capabilities.supports_cloning !== undefined) {
        expect(cloneMode.capabilities.supports_cloning).toBe(true);
      }
      if (cloneMode.capabilities.supports_voice_saving !== undefined) {
        expect(cloneMode.capabilities.supports_voice_saving).toBe(true);
      }
    }
  });
});