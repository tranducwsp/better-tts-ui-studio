import type { RangeConstraint, UniversalManifest } from './types';

/**
 * Single source for the numeric ranges behind the speed and pitch controls.
 *
 * These used to be read inline in the panel — twelve `manifest?.constraints?...` chains,
 * each fallback written twice because a control renders as either a slider or a number
 * input. Changing one and missing its twin gave the two widgets different bounds, with
 * nothing to catch it.
 *
 * The fallbacks also used `||`, which treats 0 as absent: an engine declaring `min: 0` for
 * pitch — perfectly reasonable — got 0.5 instead. Every resolver here checks for a number
 * rather than for truthiness.
 */

/** Applied when the manifest states no speed range. */
export const DEFAULT_SPEED_RANGE: RangeConstraint = {
  min: 0.5,
  max: 2.0,
  default: 1.0,
  step: 0.1,
};

/** Applied when the manifest states no pitch range. */
export const DEFAULT_PITCH_RANGE: RangeConstraint = {
  min: -10,
  max: 10,
  default: 0,
  step: 0.5,
};

function num(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function resolve(
  declared: Partial<RangeConstraint> | undefined,
  defaults: RangeConstraint
): RangeConstraint {
  const min = num(declared?.min, defaults.min);
  const max = num(declared?.max, defaults.max);
  const step = num(declared?.step, defaults.step);

  // A default outside its own range would leave the control showing a value it cannot
  // return to once moved, so clamp rather than trust the declaration.
  const rawDefault = num(declared?.default, defaults.default);
  const lo = Math.min(min, max);
  const hi = Math.max(min, max);

  return {
    min: lo,
    max: hi,
    step: step > 0 ? step : defaults.step,
    default: Math.min(hi, Math.max(lo, rawDefault)),
  };
}

/** Bounds for the speed control, with every field guaranteed present. */
export function speedRange(manifest: UniversalManifest | null | undefined): RangeConstraint {
  return resolve(manifest?.constraints?.speed_range, DEFAULT_SPEED_RANGE);
}

/** Bounds for the pitch control, with every field guaranteed present. */
export function pitchRange(manifest: UniversalManifest | null | undefined): RangeConstraint {
  return resolve(manifest?.constraints?.pitch_range, DEFAULT_PITCH_RANGE);
}
