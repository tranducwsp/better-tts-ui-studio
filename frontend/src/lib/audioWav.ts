// resampleAudioBuffer chuyển đổi AudioBuffer về sample rate mục tiêu bằng OfflineAudioContext.
// Engine thường yêu cầu 24kHz, nhưng file nguồn có thể là 48kHz — gửi sai sample rate khiến
// engine phát âm thanh nhanh/chậm hoặc từ chối. OfflineAudioContext resample chất lượng cao
// (linear interpolation), không cần thư viện ngoài.
export async function resampleAudioBuffer(buffer: AudioBuffer, targetSampleRate: number): Promise<AudioBuffer> {
  if (buffer.sampleRate === targetSampleRate) return buffer;
  const ctx = new OfflineAudioContext(buffer.numberOfChannels, Math.ceil(buffer.duration * targetSampleRate), targetSampleRate);
  const source = ctx.createBufferSource();
  source.buffer = buffer;
  source.connect(ctx.destination);
  source.start(0);
  return ctx.startRendering();
}

export function audioBufferToWav(buffer: AudioBuffer, startSec: number, endSec: number): Blob {
  const sampleRate = buffer.sampleRate;
  const startFrame = Math.floor(startSec * sampleRate);
  const endFrame = Math.floor(endSec * sampleRate);
  const numFrames = Math.max(0, endFrame - startFrame);
  const numChannels = buffer.numberOfChannels;

  // Create WAV header + PCM 16bit data
  const dataSize = numFrames * numChannels * 2;
  const bufferSize = 44 + dataSize;
  const arrayBuffer = new ArrayBuffer(bufferSize);
  const view = new DataView(arrayBuffer);

  const writeString = (offset: number, str: string) => {
    for (let i = 0; i < str.length; i++) {
      view.setUint8(offset + i, str.charCodeAt(i));
    }
  };

  /* RIFF identifier */
  writeString(0, 'RIFF');
  /* RIFF chunk length */
  view.setUint32(4, 36 + dataSize, true);
  /* RIFF type */
  writeString(8, 'WAVE');
  /* format chunk identifier */
  writeString(12, 'fmt ');
  /* format chunk length */
  view.setUint32(16, 16, true);
  /* sample format (raw PCM) */
  view.setUint16(20, 1, true);
  /* channel count */
  view.setUint16(22, numChannels, true);
  /* sample rate */
  view.setUint32(24, sampleRate, true);
  /* byte rate (sample rate * block align) */
  view.setUint32(28, sampleRate * numChannels * 2, true);
  /* block align (channel count * bytes per sample) */
  view.setUint16(32, numChannels * 2, true);
  /* bits per sample */
  view.setUint16(34, 16, true);
  /* data chunk identifier */
  writeString(36, 'data');
  /* data chunk length */
  view.setUint32(40, dataSize, true);

  // Write PCM audio samples
  let offset = 44;
  for (let i = 0; i < numFrames; i++) {
    for (let ch = 0; ch < numChannels; ch++) {
      const channelData = buffer.getChannelData(ch);
      const sampleIndex = startFrame + i;
      let sample = sampleIndex < channelData.length ? channelData[sampleIndex] : 0;
      // Clamp between -1 and 1
      sample = Math.max(-1, Math.min(1, sample));
      // Convert to 16-bit signed PCM
      view.setInt16(offset, sample < 0 ? sample * 0x8000 : sample * 0x7fff, true);
      offset += 2;
    }
  }

  return new Blob([arrayBuffer], { type: 'audio/wav' });
}
