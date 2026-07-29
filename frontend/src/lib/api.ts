import type { UserResponse, VoiceOption, Preset, HistoryItem, UniversalManifest } from './types';

export async function fetchManifest(): Promise<UniversalManifest | null> {
  try {
    const res = await fetch('/api/info', { credentials: 'include' });
    if (!res.ok) return null;
    return await res.json();
  } catch (err) {
    console.error('Lỗi fetchManifest:', err);
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
    throw new Error(data.detail || 'Đăng nhập thất bại');
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
    throw new Error(data.detail || 'Đăng ký thất bại');
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
  if (!res.ok) throw new Error('Không thể lấy danh sách người dùng');
  return await res.json();
}

export async function approveUser(userId: string): Promise<void> {
  const res = await fetch(`/api/admin/users/${userId}/approve`, {
    method: 'POST',
    credentials: 'include',
  });
  if (!res.ok) throw new Error('Không thể duyệt người dùng');
}

export function parseVoiceItem(v: any): VoiceOption {
  if (Array.isArray(v)) {
    const optionText = v[0] || '';
    const optionValue = v[1] || optionText || '';
    let name = optionText;
    let gender = '';
    let region = '';
    let style = '';

    if (optionText.includes('—')) {
      const parts = optionText.split('—').map((s: string) => s.trim());
      name = parts[0];
      if (parts[1]) {
        const tags = parts[1].split('·').map((s: string) => s.trim());
        gender = tags[0] || '';
        region = tags[1] || '';
        style = tags[2] || '';
      }
    } else {
      gender = name.includes('Nữ') ? 'Nữ' : 'Nam';
      region = name.includes('Nam') ? 'Miền Nam' : (name.includes('Trung') ? 'Miền Trung' : 'Miền Bắc');
    }

    return {
      id: optionValue,
      name: name,
      gender: gender,
      region: region,
      description: style,
      sampleUrl: v[2],
      type: 'standard'
    };
  }

  const id = v.id || v.voice_id || v.name || v;
  const rawName = v.name || v.id || v;
  let name = rawName;
  let gender = v.gender || '';
  let region = v.region || '';
  let style = v.style || v.description || '';

  if (typeof rawName === 'string' && rawName.includes('—')) {
    const parts = rawName.split('—').map((s: string) => s.trim());
    name = parts[0];
    if (parts[1]) {
      const tags = parts[1].split('·').map((s: string) => s.trim());
      if (!gender) gender = tags[0] || '';
      if (!region) region = tags[1] || '';
      if (!style) style = tags[2] || '';
    }
  }

  return {
    id: id,
    name: name,
    gender: gender || (name.includes('Nữ') ? 'Nữ' : 'Nam'),
    region: region || 'Miền Bắc',
    description: style,
    sampleUrl: v.sampleUrl || (Array.isArray(v) && v[2] ? v[2] : undefined),
    type: v.type || 'standard'
  };
}

export async function fetchVoices(modelId: string = 'standard'): Promise<VoiceOption[]> {
  try {
    const res = await fetch(`/api/voices/${modelId}`, { credentials: 'include' });
    if (!res.ok) throw new Error('Không thể tải giọng');
    const data = await res.json();
    if (Array.isArray(data)) {
      return data.map(parseVoiceItem);
    }
    if (data.voices && Array.isArray(data.voices)) {
      return data.voices.map(parseVoiceItem);
    }
    return [];
  } catch (err) {
    console.error('Lỗi fetchVoices:', err);
    return [];
  }
}

export async function fetchPresets(): Promise<Preset[]> {
  try {
    const res = await fetch('/api/clone/voices', { credentials: 'include' });
    if (!res.ok) return [];
    const data = await res.json();
    if (Array.isArray(data)) {
      return data.map((v: any) => ({
        id: v.id || v.name || v,
        name: v.name || v,
        speaker: v.name || v,
        speed: 1.0,
        gender: v.gender,
        region: v.region,
        style: v.style
      }));
    }
    return data.presets || [];
  } catch (err) {
    console.error('Lỗi fetchPresets:', err);
    return [];
  }
}

export async function deleteCloneVoice(id: string): Promise<void> {
  const res = await fetch(`/api/clone/voices/${id}`, {
    method: 'DELETE',
    credentials: 'include',
  });
  if (!res.ok) throw new Error('Không thể xóa giọng mẫu');
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
  if (!res.ok) throw new Error(data.detail || 'Lỗi đọc file');
  return data.text || '';
}

export async function synthesize(
  text: string,
  voice: string,
  speed: number,
  engine: string = 'standard',
  jobId?: string,
  chunkIndex: number = 0,
  totalChunks: number = 1
): Promise<string> {
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
    }),
    credentials: 'include',
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.detail || 'Lỗi tổng hợp âm thanh');
  return data.task_id || data.id || '';
}

export async function synthesizeStandard(
  text: string,
  voice: string,
  speed: number,
  jobId?: string,
  chunkIndex: number = 0,
  totalChunks: number = 1
): Promise<string> {
  return synthesize(text, voice, speed, 'standard', jobId, chunkIndex, totalChunks);
}

export async function synthesizeFast(
  text: string,
  voice: string,
  speed: number,
  jobId?: string,
  chunkIndex: number = 0,
  totalChunks: number = 1
): Promise<string> {
  return synthesize(text, voice, speed, 'fast', jobId, chunkIndex, totalChunks);
}

export async function synthesizeClone(
  text: string,
  voice: string,
  speed: number,
  jobId?: string,
  chunkIndex: number = 0,
  totalChunks: number = 1
): Promise<string> {
  return synthesize(text, voice, speed, 'clone', jobId, chunkIndex, totalChunks);
}

export async function cloneVoice(file: File, name: string, gender = 'Nam', region = 'Miền Bắc', style = 'Truyền cảm'): Promise<string> {
  const formData = new FormData();
  formData.append('file', file);
  formData.append('name', name);
  formData.append('gender', gender);
  formData.append('region', region);
  formData.append('style', style);
  const res = await fetch('/api/clone/upload', {
    method: 'POST',
    body: formData,
    credentials: 'include',
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.detail || 'Lỗi clone giọng');
  return data.id || data.clone_id || '';
}

export async function cloneVoiceTemp(file: File): Promise<string> {
  const formData = new FormData();
  formData.append('file', file);
  const res = await fetch('/api/clone/upload-temp', {
    method: 'POST',
    body: formData,
    credentials: 'include',
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.detail || 'Lỗi tải giọng tạm');
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
          onError('Không thể tải file audio .wav từ máy chủ');
          return;
        }
        const blob = await audioRes.blob();
        const mp3Url = `/api/tasks/${taskId}/audio?format=mp3`;
        onComplete(blob, mp3Url);
      } else if (data.status === 'error' || data.status === 'failed') {
        eventSource.close();
        onError(data.error || 'Lỗi xử lý AI từ server');
      } else if (data.status === 'cancelled') {
        eventSource.close();
        onError('Tác vụ đã bị hủy');
      }
    } catch (err: any) {
      console.error('Lỗi parse SSE task stream:', err);
    }
  };

  eventSource.onerror = (err) => {
    eventSource.close();
    onError('Lỗi kết nối luồng âm thanh Stream SSE từ máy chủ');
  };

  return () => {
    eventSource.close();
  };
}

export async function fetchHistory(): Promise<HistoryItem[]> {
  const res = await fetch('/api/history', { credentials: 'include' });
  if (!res.ok) throw new Error('Không thể tải lịch sử');
  return await res.json();
}
