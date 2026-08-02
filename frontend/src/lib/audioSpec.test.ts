import { describe, expect, it } from 'vitest';

import {
  audioSpecLabel,
  defaultFormat,
  downloadFormats,
  maxUploadBytes,
  referenceAudioSeconds,
  resolveAudioSpec,
} from './audioSpec';
import type { UniversalManifest } from './types';

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
  // The bug this exists for: fast is backed by Edge TTS and returns MP3, but a single
  // engine-wide default_format labelled it WAV, so downloads carried the wrong extension
  // and Content-Type.
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

  it('labels each mode with its own format', () => {
    expect(audioSpecLabel(manifest, 'fast')).toBe('MP3 24kHz');
    expect(audioSpecLabel(manifest, 'standard')).toBe('WAV 24kHz');
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
    expect(maxUploadBytes(null)).toBe(10 * 1024 * 1024);
    expect(referenceAudioSeconds(null)).toBe(5);
  });

  it('ignores an unknown mode id rather than erroring', () => {
    expect(defaultFormat(manifest, 'no_such_mode')).toBe('wav');
  });
});
