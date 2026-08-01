import type { UserResponse, VoiceOption, Preset, HistoryItem, UniversalManifest, JobDetailResponse, ChunkItemResponse } from './types';
export type { UserResponse, VoiceOption, Preset, HistoryItem, UniversalManifest, JobDetailResponse, ChunkItemResponse };

export async function fetchManifest(): Promise<UniversalManifest | null> {
  try {
    const res = await fetch('/api/info', { credentials: 'include' });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('fetchManifest error:', err);
    return null;
  }
}

export async function checkCurrentUser(): Promise<UserResponse | null> {
  try {
    const res = await fetch('/api/me', { credentials: 'include' });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    return null;
  }
}

export async function loginUser(username: string, password: string): Promise<UserResponse> {
  const res = await fetch('/api/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
    credentials: 'include',
  });

  const data = await res.json();
  if (!res.ok) {
    throw new Error(data.detail || 'Login failed');
  }
  return data.user || data;
}

export async function registerUser(username: string, password: string): Promise<UserResponse> {
  const res = await fetch('/api/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
    credentials: 'include',
  });

  const data = await res.json();
  if (!res.ok) {
    throw new Error(data.detail || 'Registration failed');
  }
  return data.user || data;
}

export { loginUser as login, registerUser as register };

export async function logout(): Promise<void> {
  await fetch('/api/logout', {
    method: 'POST',
    credentials: 'include',
  });
}

export async function fetchAdminUsers(): Promise<UserResponse[]> {
  const res = await fetch('/api/admin/users', { credentials: 'include' });
  if (!res.ok) throw new Error('Failed to fetch user list');
  return await res.json();
}

export async function approveUser(userId: string): Promise<void> {
  const res = await fetch(`/api/admin/users/${userId}/approve`, {
    method: 'POST',
    credentials: 'include',
  });
  if (!res.ok) throw new Error('Failed to approve user');
}

export function parseVoiceItem(v: Record<string, unknown> | string): VoiceOption {
  let descriptions: string[] = [];
  if (typeof v === 'object' && v !== null) {
    if (Array.isArray(v.descriptions)) {
      descriptions = v.descriptions.filter((d: unknown): d is string => typeof d === 'string' && d.trim() !== '');
    }
    const idStr = String(v.id || v.voice_id || v.name || '');
    const nameStr = String(v.name || v.id || '');
    const sampleUrl = typeof v.sampleUrl === 'string' ? v.sampleUrl : undefined;
    return {
      id: idStr,
      name: nameStr,
      descriptions,
      sampleUrl
    };
  }

  const str = String(v);
  return {
    id: str,
    name: str,
    descriptions: []
  };
}

export async function fetchVoices(modelId: string = 'standard'): Promise<VoiceOption[]> {
  try {
    const res = await fetch(`/api/voices/${modelId}`, { credentials: 'include' });
    if (!res.ok) throw new Error('Failed to load voices');
    const data = await res.json();
    if (Array.isArray(data)) {
      return data.map(parseVoiceItem);
    }
    if (data.voices && Array.isArray(data.voices)) {
      return data.voices.map(parseVoiceItem);
    }
    return [];
  } catch (err) {
    console.error('fetchVoices error:', err);
    return [];
  }
}

export async function fetchPresets(modelId?: string): Promise<Preset[]> {
  try {
    const url = modelId ? `/api/clone/voices?model_id=${encodeURIComponent(modelId)}` : '/api/clone/voices';
    const res = await fetch(url, { credentials: 'include' });
    if (!res.ok) return [];
    const data = await res.json();
    if (Array.isArray(data)) {
      return data.map((v: Record<string, unknown>) => ({
        id: String(v.id || v.name || ''),
        name: String(v.name || ''),
        speaker: String(v.name || ''),
        speed: 1.0,
        gender: typeof v.gender === 'string' ? v.gender : undefined,
        region: typeof v.region === 'string' ? v.region : undefined,
        style: typeof v.style === 'string' ? v.style : undefined,
        created_at: typeof v.created_at === 'string' ? v.created_at : undefined
      }));
    }
    return data.presets || [];
  } catch (err) {
    console.error('fetchPresets error:', err);
    return [];
  }
}

export async function deleteCloneVoice(id: string): Promise<void> {
  const res = await fetch(`/api/clone/voices/${id}`, {
    method: 'DELETE',
    credentials: 'include',
  });
  if (!res.ok) throw new Error('Failed to delete voice sample');
}

export async function extractTextFromFile(file: File): Promise<string> {
  const formData = new FormData();
  formData.append('file', file);
  const res = await fetch('/api/extract-text', {
    method: 'POST',
    body: formData,
    credentials: 'include',
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.detail || 'Error reading file');
  return data.text || '';
}

export interface SynthesizeOptions {
  jobId?: string;
  chunkIndex?: number;
  totalChunks?: number;
  pitch?: number;
  emotion?: string;
}

export async function synthesize(
  text: string,
  voice: string,
  speed: number,
  engine: string = 'standard',
  options: SynthesizeOptions = {}
): Promise<string> {
  const { jobId, chunkIndex = 0, totalChunks = 1, pitch, emotion } = options;
  const res = await fetch(`/api/synthesize/${engine}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      text,
      voice,
      speed,
      engine,
      job_id: jobId,
      chunk_index: chunkIndex,
      total_chunks: totalChunks,
      // Only sent when the manifest declares the engine supports them
      ...(pitch !== undefined ? { pitch } : {}),
      ...(emotion ? { emotion } : {}),
    }),
    credentials: 'include',
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.detail || 'Audio synthesis failed');
  return data.task_id || data.id || '';
}



export async function cloneVoice(file: File, name: string, gender = 'Male', region = 'Northern', style = 'Expressive', modelId = 'clone'): Promise<string> {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('name', name);
  formData.append('gender', gender);
  formData.append('region', region);
  formData.append('style', style);
  formData.append('model_id', modelId);
  const res = await fetch('/api/clone/upload', {
    method: 'POST',
    body: formData,
    credentials: 'include',
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.detail || 'Voice cloning failed');
  return data.id || data.clone_id || '';
}

export async function cloneVoiceTemp(file: File, modelId = 'clone'): Promise<string> {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('model_id', modelId);
  const res = await fetch('/api/clone/upload-temp', {
    method: 'POST',
    body: formData,
    credentials: 'include',
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.detail || 'Failed to upload temporary voice');
  return data.id || data.clone_id || '';
}

export function subscribeTaskStream(
  taskId: string,
  onProgress: (progress: number) => void,
  onComplete: (blob: Blob, mp3Url: string) => void,
  onError: (errorMsg: string) => void
): () => void {
  const eventSource = new EventSource(`/api/stream/tasks/${taskId}`, { withCredentials: true });

  eventSource.onmessage = async (e) => {
    try {
      const data = JSON.parse(e.data);

      if (data.status === 'processing') {
        onProgress(data.progress || 0);
      } else if (data.status === 'done') {
        eventSource.close();
        const audioRes = await fetch(`/api/tasks/${taskId}/audio?format=wav`, { credentials: 'include' });
        if (!audioRes.ok) {
          onError('Failed to load audio file .wav from server');
          return;
        }
        const blob = await audioRes.blob();
        const mp3Url = `/api/tasks/${taskId}/audio?format=mp3`;
        onComplete(blob, mp3Url);
      } else if (data.status === 'error' || data.status === 'failed') {
        eventSource.close();
        onError(data.error || 'AI processing error from server');
      } else if (data.status === 'cancelled') {
        eventSource.close();
        onError('Task was cancelled');
      }
    } catch (err: unknown) {
      console.error('Error parsing SSE task stream:', err);
    }
  };

  eventSource.onerror = (err) => {
    eventSource.close();
    onError('Error connecting to Audio Stream SSE from server');
  };

  return () => {
    eventSource.close();
  };
}

export async function fetchHistory(): Promise<HistoryItem[]> {
  const res = await fetch('/api/history', { credentials: 'include' });
  if (!res.ok) throw new Error('Failed to fetch history');
  return await res.json();
}
