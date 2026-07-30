export interface VoiceOption {
  id: string;
  name: string;
  descriptions?: string[];
  sampleUrl?: string;
}

export interface Preset {
  id: string;
  name: string;
  speaker: string;
  speed: number;
  gender?: string;
  region?: string;
  style?: string;
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

export interface UserResponse {
  id: string;
  username: string;
  role: string;
  is_approved: boolean;
}

export interface EngineModeSpec {
  id: string;
  name: string;
  description: string;
  supports_preset_voices?: boolean;
  supports_cloning?: boolean;
  supports_voice_saving?: boolean;
  supports_streaming?: boolean;
}

export interface RangeConstraint {
  min: number;
  max: number;
  default: number;
  step: number;
}

export interface EngineConstraints {
  max_text_length: number;
  speed_range: RangeConstraint;
  pitch_range: RangeConstraint;
  supported_emotions: string[];
}

export interface AudioSpec {
  supported_formats: string[];
  supported_sample_rates: number[];
  default_format: string;
  default_sample_rate: number;
}

export interface EngineCapabilities {
  supports_preset_voices: boolean;
  supports_cloning: boolean;
  supports_streaming: boolean;
  supports_speed: boolean;
  supports_pitch: boolean;
  supports_emotion: boolean;
  supports_ssml: boolean;
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
  max_chunk_size?: number;
  chunk_delimiters?: string[];
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

export interface ModelOptionSpec {
  notice_banner?: NoticeBannerSpec;
  voice_type?: string;
  speed_type?: string;
  pitch_type?: string;
  emotion_type?: string;
  preset_voices?: Array<{ id: string; name: string; gender?: string }>;
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
