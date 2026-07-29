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

export interface UniversalManifest {
  engine_id: string;
  engine_name: string;
  version: string;
  provider: string;
  supported_modes: EngineModeSpec[];
  capabilities: EngineCapabilities;
  constraints: EngineConstraints;
  audio_spec: AudioSpec;
}
