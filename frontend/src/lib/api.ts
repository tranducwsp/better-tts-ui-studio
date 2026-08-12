import type { HistoryPage, UserResponse, VoiceOption, Preset, HistoryItem, UniversalManifest, JobDetailResponse, ChunkItemResponse } from './types';
export type { HistoryPage, UserResponse, VoiceOption, Preset, HistoryItem, UniversalManifest, JobDetailResponse, ChunkItemResponse };

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

export async function refreshSession(): Promise<boolean> {
  // Deduplicate: if a refresh is already in flight, all callers share the same Promise.
  // Without this, 10 concurrent 401 requests → 10 separate refreshes → token thrashing.
  if (refreshInFlight) return refreshInFlight;
  refreshInFlight = doRefresh();
  try {
    return await refreshInFlight;
  } finally {
    refreshInFlight = null;
  }
}

let refreshInFlight: Promise<boolean> | null = null;

async function doRefresh(): Promise<boolean> {
  try {
    let res = await fetch('/api/auth/refresh', {
      method: 'POST',
      credentials: 'include',
    });
    // Rotation: if another tab just rotated before, the cookie jar already has the newest token
    // but this request carried the old token and got 401 ("already used"). Retrying once with the
    // current cookie fixes it; a second 401 is a genuinely dead session.
    if (res.status === 401) {
      res = await fetch('/api/auth/refresh', {
        method: 'POST',
        credentials: 'include',
      });
    }
    return res.ok;
  } catch {
    return false;
  }
}

// authFetch works like fetch but automatically retries refreshSession() once on 401.
// Every API call that needs authentication should use this instead of bare fetch() — access tokens
// last 15 minutes, and sessions longer than 15 minutes without retry will get 401 "Please log in"
// even though the refresh token is still valid.
export async function authFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  let res = await fetch(input, { ...init, credentials: 'include' });
  if (res.status === 401 && await refreshSession()) {
    res = await fetch(input, { ...init, credentials: 'include' });
  }
  return res;
}

export async function checkCurrentUser(): Promise<UserResponse | null> {
  try {
    const res = await authFetch('/api/me');
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

  if (!res.ok) {
    let detail = 'Login failed';
    try { const data = await res.json(); detail = data.detail || detail; } catch {}
    throw new Error(detail);
  }
  const data = await res.json();
  return data.user || data;
}

export async function registerUser(username: string, password: string): Promise<UserResponse> {
  const res = await fetch('/api/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
    credentials: 'include',
  });

  if (!res.ok) {
    let detail = 'Registration failed';
    try { const data = await res.json(); detail = data.detail || detail; } catch {}
    throw new Error(detail);
  }
  const data = await res.json();
  return data.user || data;
}

export { loginUser as login, registerUser as register };

export async function logout(): Promise<void> {
  await authFetch('/api/auth/logout', { method: 'POST' });
}

export async function fetchAdminUsers(): Promise<UserResponse[]> {
  const res = await authFetch('/api/admin/users');
  if (!res.ok) throw new Error('Failed to fetch user list');
  return await res.json();
}

export async function approveUser(userId: string): Promise<void> {
  const res = await authFetch(`/api/admin/users/${userId}/approve`, {
    method: 'POST',
  });
  if (!res.ok) throw new Error('Failed to approve user');
}

export function parseVoiceItem(v: Record<string, unknown> | string): VoiceOption {
  if (typeof v === 'object' && v !== null) {
    const idStr = String(v.id || v.voice_id || v.name || '');
    const nameStr = String(v.name || v.id || '');
    const sampleUrl = typeof v.sampleUrl === 'string' ? v.sampleUrl : (typeof v.sample_url === 'string' ? v.sample_url : undefined);
    const metadata = typeof v.metadata === 'object' && v.metadata !== null ? (v.metadata as Record<string, string>) : undefined;

    const deletable = typeof v.deletable === 'boolean' ? v.deletable : undefined;
    return {
      id: idStr,
      name: nameStr,
      metadata,
      sampleUrl,
      deletable
    };
  }

  const str = String(v);
  return {
    id: str,
    name: str,
  };
}

export async function fetchVoices(modelId: string): Promise<VoiceOption[]> {
  try {
    const res = await authFetch(`/api/voices/${modelId}`);
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
    const res = await authFetch(url);
    if (!res.ok) return [];
    const data = await res.json();
    if (Array.isArray(data)) {
      return data.map((v: Record<string, unknown>) => ({
        id: String(v.id || v.name || ''),
        name: String(v.name || ''),
        speed: 1.0,
        metadata: typeof v.metadata === 'object' && v.metadata !== null
          ? (v.metadata as Record<string, string>)
          : undefined,
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
  const res = await authFetch(`/api/clone/voices/${id}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw new Error('Failed to delete voice sample');
}

export async function extractTextFromFile(file: File): Promise<string> {
  const formData = new FormData();
  formData.append('file', file);
  const res = await authFetch('/api/extract-text', {
    method: 'POST',
    body: formData,
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
  engine: string,
  options: SynthesizeOptions = {}
): Promise<string> {
  const { jobId, chunkIndex = 0, totalChunks = 1, pitch, emotion } = options;
  const res = await authFetch(`/api/synthesize/${engine}`, {
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
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.detail || 'Audio synthesis failed');
  return data.task_id || data.id || '';
}



/**
 * Upload a reference clip and save it as a named voice.
 *
 * `fields` is whatever the engine's voice_metadata_schema asked for, passed through as-is.
 * gender/region/style land in dedicated columns because the platform filters and displays
 * them; anything else the engine declares goes into a JSONB column. Sending a fixed three
 * would silently drop the rest — an engine declaring five fields would see two vanish.
 */
export async function cloneVoice(
  file: File,
  name: string,
  fields: Record<string, string>,
  modelId: string
): Promise<string> {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('name', name);
  formData.append('model_id', modelId);
  for (const [key, value] of Object.entries(fields)) {
    if (key === 'name' || value === '') continue;
    formData.append(key, value);
  }

  const res = await authFetch('/api/clone/upload', {
    method: 'POST',
    body: formData,
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.detail || 'Voice cloning failed');
  return data.id || data.clone_id || '';
}

/** modelId identifies which mode the reference audio belongs to; callers pass activeMode.id. */
export async function cloneVoiceTemp(file: File, modelId = ''): Promise<string> {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('model_id', modelId);
  const res = await authFetch('/api/clone/upload-temp', {
    method: 'POST',
    body: formData,
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.detail || 'Failed to upload temporary voice');
  return data.id || data.clone_id || '';
}

export function subscribeTaskStream(
  taskId: string,
  onProgress: (progress: number) => void,
  onComplete: (blob: Blob, altUrl: string) => void,
  onError: (errorMsg: string) => void,
  format = 'wav'
): () => void {
  const eventSource = new EventSource(`/api/stream/tasks/${taskId}`, { withCredentials: true });

  eventSource.onmessage = async (e) => {
    try {
      const data = JSON.parse(e.data);

      if (data.status === 'processing') {
        onProgress(data.progress || 0);
      } else if (data.status === 'done') {
        eventSource.close();
        const audioRes = await authFetch(`/api/tasks/${taskId}/audio?format=${encodeURIComponent(format)}`);
        if (!audioRes.ok) {
          onError(`Failed to load audio file .${format} from server`);
          return;
        }
        const blob = await audioRes.blob();
        // altUrl: URL for the same audio in a different format (first non-default from
        // supported_formats).  Kept for backward compat — current callers ignore it.
        const altUrl = `/api/tasks/${taskId}/audio?format=${format === 'mp3' ? 'wav' : 'mp3'}`;
        onComplete(blob, altUrl);
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

  // D6: SSE onerror does not close immediately — the browser auto-reconnects when readyState is
  // CONNECTING (0). Only call onError when the connection is permanently closed (readyState =
  // CLOSED / 2), i.e. the server returned 404/403 or an unrecoverable error. If the network is
  // temporarily lost, EventSource reconnects automatically; the backend sends a status snapshot
  // when SSE reopens, so no progress is lost. Previously onerror closed immediately → retry
  // created a new task, discarding 90% of what was already done.
  eventSource.onerror = () => {
    if (eventSource.readyState === EventSource.CLOSED) {
      onError('Connection to Audio Stream closed permanently');
    }
    // readyState === CONNECTING: browser is auto-reconnecting — do nothing.
  };

  return () => {
    eventSource.close();
  };
}

// HistoryCursor is the stopping point of the previous page to get the next page: created_at and
// job_id of the last item. Pass null for the first page.
export interface HistoryCursor {
  created_at: string;
  job_id: string;
}

export async function fetchHistory(before: HistoryCursor | null = null): Promise<HistoryPage> {
  const params = new URLSearchParams();
  if (before) {
    params.set('before', before.created_at);
    params.set('before_id', before.job_id);
  }
  const qs = params.toString();
  const res = await authFetch(`/api/history${qs ? `?${qs}` : ''}`);
  if (!res.ok) throw new Error('Failed to fetch history');
  return await res.json();
}
