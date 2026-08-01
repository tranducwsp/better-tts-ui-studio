import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { PLATFORM_DEFAULTS, resolveCapabilities } from './capabilities';
import { DEFAULT_TEXT_LIMIT, resolveChunkSize } from './textLimits';
import type { UniversalManifest } from './types';

/**
 * The same fixture core-backend/tests/parity_test.go reads.
 *
 * resolveCapabilities() and types.ResolveCapabilities() implement one rule in two
 * languages, and nothing in either toolchain notices when they stop agreeing. Driving both
 * from one file is what makes a drift show up as a failing test rather than as a control
 * that appears in the UI but is rejected by the backend.
 */
const fixture = JSON.parse(
  readFileSync(resolve(__dirname, '../../../docs/capability-resolution-cases.json'), 'utf-8')
) as {
  manifest: UniversalManifest;
  cases: { why: string; mode: string; expect: Record<string, boolean> }[];
  platform_defaults: Record<string, boolean>;
  default_max_text_length: number;
};

describe('resolveCapabilities', () => {
  it('has cases to run', () => {
    expect(fixture.cases.length).toBeGreaterThan(0);
  });

  for (const c of fixture.cases) {
    it(`${c.mode || '(empty id)'}: ${c.why}`, () => {
      const got = resolveCapabilities(fixture.manifest, c.mode) as unknown as Record<
        string,
        boolean
      >;
      for (const [key, want] of Object.entries(c.expect)) {
        expect(got[key], key).toBe(want);
      }
    });
  }

  it('accepts a mode object as well as an id', () => {
    const spec = fixture.manifest.supported_modes.find((m) => m.id === 'opts_in')!;
    expect(resolveCapabilities(fixture.manifest, spec).supports_pitch).toBe(true);
  });

  it('falls back to platform defaults with no manifest at all', () => {
    const got = resolveCapabilities(null, 'anything') as unknown as Record<string, boolean>;
    for (const [key, want] of Object.entries(fixture.platform_defaults)) {
      expect(got[key], key).toBe(want);
    }
  });
});

describe('platform defaults agree with the shared fixture', () => {
  it('capability defaults', () => {
    const got = PLATFORM_DEFAULTS as unknown as Record<string, boolean>;
    for (const [key, want] of Object.entries(fixture.platform_defaults)) {
      expect(got[key], key).toBe(want);
    }
  });

  it('text length default', () => {
    expect(DEFAULT_TEXT_LIMIT).toBe(fixture.default_max_text_length);
    expect(resolveChunkSize(null)).toBe(fixture.default_max_text_length);
  });
});
