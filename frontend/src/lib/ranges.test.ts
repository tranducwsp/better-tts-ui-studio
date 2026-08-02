import { describe, expect, it } from 'vitest';

import { DEFAULT_PITCH_RANGE, DEFAULT_SPEED_RANGE, pitchRange, speedRange } from './ranges';
import type { UniversalManifest } from './types';

const mf = (constraints: unknown): UniversalManifest =>
  ({ constraints }) as unknown as UniversalManifest;

describe('speedRange', () => {
  it('uses what the engine declares', () => {
    const r = speedRange(mf({ speed_range: { min: 0.25, max: 4, default: 1.5, step: 0.25 } }));
    expect(r).toEqual({ min: 0.25, max: 4, default: 1.5, step: 0.25 });
  });

  it('falls back when the manifest is absent', () => {
    expect(speedRange(null)).toEqual(DEFAULT_SPEED_RANGE);
    expect(speedRange(mf({}))).toEqual(DEFAULT_SPEED_RANGE);
  });

  it('fills in only the missing fields', () => {
    const r = speedRange(mf({ speed_range: { max: 3 } }));
    expect(r.max).toBe(3);
    expect(r.min).toBe(DEFAULT_SPEED_RANGE.min);
    expect(r.step).toBe(DEFAULT_SPEED_RANGE.step);
  });
});

describe('pitchRange', () => {
  // The bug this module was extracted to fix: the panel used `|| -10`, and 0 is falsy, so
  // an engine declaring a floor of 0 silently got -10 and the slider offered pitches the
  // engine had ruled out.
  it('keeps a declared zero instead of treating it as absent', () => {
    const r = pitchRange(mf({ pitch_range: { min: 0, max: 12, default: 0, step: 1 } }));
    expect(r.min).toBe(0);
    expect(r.default).toBe(0);
  });

  it('falls back when the manifest is absent', () => {
    expect(pitchRange(null)).toEqual(DEFAULT_PITCH_RANGE);
  });
});

describe('malformed declarations', () => {
  it('orders inverted bounds rather than producing an unusable control', () => {
    const r = speedRange(mf({ speed_range: { min: 3, max: 1 } }));
    expect(r.min).toBe(1);
    expect(r.max).toBe(3);
  });

  it('clamps a default that sits outside its own range', () => {
    // Left alone, the control would open on a value it could never return to.
    expect(speedRange(mf({ speed_range: { min: 1, max: 2, default: 9 } })).default).toBe(2);
    expect(speedRange(mf({ speed_range: { min: 1, max: 2, default: 0 } })).default).toBe(1);
  });

  it('rejects a non-positive step', () => {
    expect(speedRange(mf({ speed_range: { step: 0 } })).step).toBe(DEFAULT_SPEED_RANGE.step);
    expect(speedRange(mf({ speed_range: { step: -1 } })).step).toBe(DEFAULT_SPEED_RANGE.step);
  });

  it('ignores non-numeric values', () => {
    const r = speedRange(mf({ speed_range: { min: 'fast', max: null, step: undefined } }));
    expect(r).toEqual(DEFAULT_SPEED_RANGE);
  });
});
