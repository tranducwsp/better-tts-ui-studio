import { describe, expect, it } from 'vitest';

import {
  acceptedUploadFormats,
  defaultFormat,
  downloadFormats,
  maxUploadBytes,
  referenceAudioSeconds,
  referenceSizeError,
  resolveAudioSpec,
} from '../../src/lib/audioSpec';
import { PLATFORM_DEFAULTS } from '../../src/lib/capabilities';
import { resolveInputPanel } from '../../src/lib/inputPanel';
import type { UniversalManifest } from '../../src/lib/types';

// Mirrors the bundled engine: WAV engine-wide, MP3 for the Edge-TTS-backed mode.
const manifest = {
  audio_spec: {
    supported_formats: ['wav', 'mp3'],
    supported_sample_rates: [24000],
    default_format: 'wav',
    default_sample_rate: 24000,
    max_upload_bytes: 10 * 1024 * 1024,
    reference_audio_seconds: 5,
  },
  supported_modes: [
    { id: 'standard', name: 'Standard', description: '' },
    {
      id: 'fast',
      name: 'Fast',
      description: '',
      audio_spec: { supported_formats: ['mp3', 'wav'], default_format: 'mp3' },
    },
  ],
} as unknown as UniversalManifest;

describe('per-mode audio spec', () => {
  it('lets a mode override the engine default format', () => {
    expect(defaultFormat(manifest, 'fast')).toBe('mp3');
    expect(defaultFormat(manifest, 'standard')).toBe('wav');
  });

  it('inherits fields the mode does not state', () => {
    const fast = resolveAudioSpec(manifest, 'fast');
    expect(fast.default_format).toBe('mp3');
    expect(fast.max_upload_bytes).toBe(10 * 1024 * 1024);
    expect(fast.reference_audio_seconds).toBe(5);
  });

  it('puts the mode default first in the download list', () => {
    expect(downloadFormats(manifest, 'fast')[0]).toBe('mp3');
    expect(downloadFormats(manifest, 'standard')[0]).toBe('wav');
  });

  it('accepts a mode object as well as an id', () => {
    const fast = manifest.supported_modes.find((m) => m.id === 'fast')!;
    expect(defaultFormat(manifest, fast)).toBe('mp3');
  });

  it('resolves engine-wide when no mode is given', () => {
    expect(defaultFormat(manifest)).toBe('wav');
  });

  it('falls back to platform defaults with no manifest', () => {
    expect(defaultFormat(null)).toBe('wav');
    expect(maxUploadBytes(null)).toBe(100 * 1024 * 1024);
    expect(referenceAudioSeconds(null)).toBe(5);
  });

  it('ignores an unknown mode id rather than erroring', () => {
    expect(defaultFormat(manifest, 'no_such_mode')).toBe('wav');
  });

  it('returns engine-wide defaults for empty mode id string', () => {
    expect(defaultFormat(manifest, '')).toBe('wav');
  });

  it('mode partial override: only some fields override engine', () => {
    const fast = resolveAudioSpec(manifest, 'fast');
    // fast overrides supported_formats AND default_format — check both
    expect(fast.supported_formats).toEqual(['mp3', 'wav']);
    expect(fast.default_format).toBe('mp3');
    // Inherited from engine
    expect(fast.default_sample_rate).toBe(24000);
  });
});

describe('resolveAudioSpec', () => {
  it('returns platform defaults when manifest has no audio_spec', () => {
    const m = { supported_modes: [] } as unknown as UniversalManifest;
    const spec = resolveAudioSpec(m);
    expect(spec.default_format).toBe('wav');
    expect(spec.default_sample_rate).toBe(24000);
    expect(spec.supported_formats).toEqual(['wav']);
    expect(spec.max_upload_bytes).toBe(100 * 1024 * 1024);
    expect(spec.max_reference_bytes).toBe(10 * 1024 * 1024);
  });

  it('empty mode AudioSpec fields do not override engine', () => {
    const m = {
      audio_spec: {
        supported_formats: ['wav', 'mp3'],
        default_format: 'wav',
        default_sample_rate: 24000,
        max_upload_bytes: 50 * 1024 * 1024,
      },
      supported_modes: [
        {
          id: 'empty',
          name: 'Empty',
          description: '',
          audio_spec: {},
        },
      ],
    } as unknown as UniversalManifest;
    const spec = resolveAudioSpec(m, 'empty');
    expect(spec.default_format).toBe('wav');
    expect(spec.supported_formats).toEqual(['wav', 'mp3']);
    expect(spec.max_upload_bytes).toBe(50 * 1024 * 1024);
  });

  it('resolves full mode override on all fields', () => {
    const m = {
      audio_spec: {
        supported_formats: ['wav'],
        supported_sample_rates: [24000],
        default_format: 'wav',
        default_sample_rate: 24000,
        reference_audio_formats: ['wav'],
        reference_audio_seconds: 5.0,
        max_upload_bytes: 100 * 1024 * 1024,
        max_reference_bytes: 10 * 1024 * 1024,
      },
      supported_modes: [
        {
          id: 'full',
          name: 'Full',
          description: '',
          audio_spec: {
            supported_formats: ['flac', 'wav'],
            supported_sample_rates: [48000],
            default_format: 'flac',
            default_sample_rate: 48000,
            reference_audio_formats: ['flac', 'wav'],
            reference_audio_seconds: 10.0,
            max_upload_bytes: 200 * 1024 * 1024,
            max_reference_bytes: 20 * 1024 * 1024,
          },
        },
      ],
    } as unknown as UniversalManifest;
    const spec = resolveAudioSpec(m, 'full');
    expect(spec.supported_formats).toEqual(['flac', 'wav']);
    expect(spec.supported_sample_rates).toEqual([48000]);
    expect(spec.default_format).toBe('flac');
    expect(spec.default_sample_rate).toBe(48000);
    expect(spec.reference_audio_formats).toEqual(['flac', 'wav']);
    expect(spec.reference_audio_seconds).toBe(10.0);
    expect(spec.max_upload_bytes).toBe(200 * 1024 * 1024);
    expect(spec.max_reference_bytes).toBe(20 * 1024 * 1024);
  });
});

describe('two upload limits', () => {
  it('uses distinct fields for raw and post-trim', () => {
    const m = {
      audio_spec: {
        reference_audio_seconds: 5,
        max_upload_bytes: 100 * 1024 * 1024,
        max_reference_bytes: 10 * 1024 * 1024,
      },
    } as unknown as UniversalManifest;
    expect(maxUploadBytes(m)).toBe(100 * 1024 * 1024);
    expect(m.audio_spec.max_upload_bytes).toBeGreaterThan(m.audio_spec.max_reference_bytes!);
  });

  it('accepts MP3 when the engine declares it, not what the engine emits', () => {
    const m = {
      audio_spec: {
        supported_formats: ['wav'],
        reference_audio_formats: ['wav', 'mp3'],
      },
    } as unknown as UniversalManifest;
    const accepted = acceptedUploadFormats(m);
    expect(accepted).toBe('.wav,.mp3');
    expect(accepted).not.toContain('.flac');
  });

  it('rejects a clip over the post-trim ceiling but not the raw one', () => {
    const m = {
      audio_spec: {
        max_upload_bytes: 100 * 1024 * 1024,
        max_reference_bytes: 10 * 1024 * 1024,
      },
    } as unknown as UniversalManifest;

    expect(referenceSizeError(9 * 1024 * 1024, m)).toBeNull();
    expect(referenceSizeError(10 * 1024 * 1024, m)).toBeNull();
    expect(referenceSizeError(11 * 1024 * 1024, m)).toContain('10 MB');
    // A 50 MB raw file is under the upload ceiling, so the picker lets it in — this is the
    // check that still stops it from reaching the engine untrimmed.
    expect(referenceSizeError(50 * 1024 * 1024, m)).not.toBeNull();
  });

  it('honours a per-mode reference ceiling', () => {
    const m = {
      audio_spec: { max_reference_bytes: 10 * 1024 * 1024 },
      supported_modes: [
        { id: 'tight', name: 'Tight', description: '', audio_spec: { max_reference_bytes: 1024 * 1024 } },
      ],
    } as unknown as UniversalManifest;
    expect(referenceSizeError(2 * 1024 * 1024, m, 'tight')).not.toBeNull();
    expect(referenceSizeError(2 * 1024 * 1024, m)).toBeNull();
  });
});
