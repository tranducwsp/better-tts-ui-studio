export interface VoiceOption {
  id: string;
  name: string;
  /** Drives the venus/mars icon in radio-style voice pickers, when the engine states it. */
  gender?: string;
  descriptions?: string[];
  sampleUrl?: string;
  createdAt?: string;
}

export interface Preset {
  id: string;
  name: string;
  speaker: string;
  speed: number;
  gender?: string;
  region?: string;
  style?: string;
  created_at?: string;
}

export interface HistoryItem {
  job_id: string;
  engine: string;
  voice: string;
  speed: number;
  text: string;
  time_ago: string;
  progress: string;
  is_complete: boolean;
}

export interface ChunkItemResponse {
  task_id: string;
  chunk_index: number;
  audio_path?: string | null;
  status: string;
  text: string;
}

export interface JobDetailResponse {
  job_id: string;
  engine: string;
  voice: string;
  speed: number;
  // Omitted by the API when the engine had no pitch/emotion control for that job.
  pitch?: number;
  emotion?: string;
  total_chunks: number;
  text: string;
  chunks: ChunkItemResponse[];
}

export interface UserResponse {
  id: string;
  username: string;
  role: string;
  is_approved: boolean;
  is_online?: boolean;
}

/**
 * What an engine or one of its modes can do.
 *
 * Every field is optional on purpose: `undefined` means "not stated, inherit", which is
 * distinct from `false` meaning "explicitly cannot". Never read these directly — call
 * `resolveCapabilities()` from lib/capabilities.ts so the precedence rule exists once.
 */
export interface EngineCapabilities {
  supports_preset_voices?: boolean;
  supports_cloning?: boolean;
  supports_voice_saving?: boolean;
  supports_streaming?: boolean;
  supports_speed?: boolean;
  supports_pitch?: boolean;
  supports_emotion?: boolean;
  supports_ssml?: boolean;
}

/** Capabilities after the mode/engine merge — no undefined left, safe to read. */
export type ResolvedCapabilities = Required<EngineCapabilities>;

export interface EngineModeSpec {
  id: string;
  name: string;
  description: string;
  capabilities?: EngineCapabilities;
}

export interface RangeConstraint {
  min: number;
  max: number;
  default: number;
  step: number;
}

/**
 * How text longer than max_text_length must be divided. Lives under constraints because it
 * governs what gets transmitted, not how anything looks.
 */
export interface ChunkingSpec {
  max_chunk_size?: number;
  /** Ordered cut points, most preferred first. Used with split(), so lookbehinds. */
  delimiters?: string[];
}

export interface EngineConstraints {
  max_text_length: number;
  speed_range: RangeConstraint;
  pitch_range: RangeConstraint;
  supported_emotions: string[];
  chunking?: ChunkingSpec;
}

export interface AudioSpec {
  supported_formats: string[];
  supported_sample_rates: number[];
  default_format: string;
  default_sample_rate: number;
  /** Ceiling for reference-audio uploads, declared by the engine that consumes them. */
  max_upload_bytes?: number;
  /** How many seconds of reference audio the engine wants for voice cloning. */
  reference_audio_seconds?: number;
}

export interface AutoFormatRule {
  find: string;
  replace: string;
}

export interface NoticeBannerSpec {
  level: 'info' | 'warning' | 'danger' | 'success' | string;
  message: string;
}

export interface InputPanelSpec {
  file_serve: boolean;
  closeable: boolean;
  find_mode: 'express' | 'expert' | string;
  replace_tool: boolean;
  enable_chunk_box: boolean;
  auto_format?: AutoFormatRule[];
}

export interface VoiceMetadataFieldSpec {
  key: string;
  label: string;
  type: 'text' | 'select' | string;
  required?: boolean;
  placeholder?: string;
  options?: string[];
}

/** A voice the engine ships with, listed in the manifest instead of via /voices. */
export interface PresetVoiceSpec {
  id: string;
  name: string;
  gender?: string;
  descriptions?: string[];
  sample_url?: string;
}

/**
 * Presentation only: which widget draws a control, never whether it exists. A `pitch_type`
 * on a mode whose resolved supports_pitch is false has no effect.
 */
export interface ModelOptionSpec {
  notice_banner?: NoticeBannerSpec;
  voice_type?: string;
  speed_type?: string;
  pitch_type?: string;
  emotion_type?: string;
  preset_voices?: PresetVoiceSpec[];
  voice_metadata_schema?: VoiceMetadataFieldSpec[];
}

export interface UISchemaSpec {
  ui_mode?: 'beauty' | 'fast' | string;
  input_panel: InputPanelSpec;
  model_sort?: string[];
  option_panel?: Record<string, ModelOptionSpec>;
}

export interface UniversalManifest {
  engine_id: string;
  engine_name: string;
  version: string;
  provider: string;
  supported_modes: EngineModeSpec[];
  capabilities: EngineCapabilities;
  constraints: EngineConstraints;
  audio_spec: AudioSpec;
  ui_schema?: UISchemaSpec;
}

export interface ToastMessage {
  id: string;
  type: 'info' | 'success' | 'warning' | 'error';
  message: string;
}

