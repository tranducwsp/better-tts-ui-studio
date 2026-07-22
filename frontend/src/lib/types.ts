export interface VoiceOption {
  id: string;
  name: string;
  gender: string;
  region: string;
  description?: string;
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
