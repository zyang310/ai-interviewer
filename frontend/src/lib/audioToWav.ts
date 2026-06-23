// Converts recorded mic audio to 16 kHz mono 16-bit PCM WAV (LINEAR16).
//
// Why: Google Speech-to-Text needs LINEAR16 and cannot ingest the AAC/MP4 that
// WKWebView's MediaRecorder produces on macOS. ElevenLabs Scribe also accepts
// WAV, so re-encoding every recording to one WAV format lets either provider
// transcribe the mic and sidesteps the codec mismatch entirely. decodeAudioData
// handles whatever the recorder emitted (AAC, Opus, …), so this works on both
// WKWebView and Chromium.

const TARGET_SAMPLE_RATE = 16000;

// blobToWavBase64 decodes a recorded audio Blob and returns it as base64-encoded
// 16 kHz mono 16-bit PCM WAV (no data-URI prefix).
export async function blobToWavBase64(blob: Blob): Promise<string> {
  const arrayBuffer = await blob.arrayBuffer();
  const decoded = await decodeAudio(arrayBuffer);
  const mono16k = await resampleToMono16k(decoded);
  const wav = encodeWav(mono16k, TARGET_SAMPLE_RATE);
  return arrayBufferToBase64(wav);
}

// decodeAudio decodes compressed audio bytes into an AudioBuffer, with a WebKit
// fallback for older WKWebView builds (the frameless overlay runs in the OS webview).
async function decodeAudio(data: ArrayBuffer): Promise<AudioBuffer> {
  const Ctx: typeof AudioContext =
    window.AudioContext || (window as any).webkitAudioContext;
  const ctx = new Ctx();
  try {
    return await ctx.decodeAudioData(data.slice(0));
  } finally {
    void ctx.close();
  }
}

// resampleToMono16k renders the buffer through an OfflineAudioContext to downmix
// to mono and resample to 16 kHz (Web Audio applies the standard down-mix when
// the destination has a single channel).
async function resampleToMono16k(buffer: AudioBuffer): Promise<Float32Array> {
  const OfflineCtx: typeof OfflineAudioContext =
    window.OfflineAudioContext || (window as any).webkitOfflineAudioContext;
  const length = Math.max(1, Math.ceil(buffer.duration * TARGET_SAMPLE_RATE));
  const offline = new OfflineCtx(1, length, TARGET_SAMPLE_RATE);
  const source = offline.createBufferSource();
  source.buffer = buffer;
  source.connect(offline.destination);
  source.start();
  const rendered = await offline.startRendering();
  return rendered.getChannelData(0);
}

// encodeWav writes a 16-bit PCM WAV (RIFF header + samples) from mono float data.
function encodeWav(samples: Float32Array, sampleRate: number): ArrayBuffer {
  const buffer = new ArrayBuffer(44 + samples.length * 2);
  const view = new DataView(buffer);
  const writeString = (offset: number, s: string) => {
    for (let i = 0; i < s.length; i++) view.setUint8(offset + i, s.charCodeAt(i));
  };

  writeString(0, "RIFF");
  view.setUint32(4, 36 + samples.length * 2, true);
  writeString(8, "WAVE");
  writeString(12, "fmt ");
  view.setUint32(16, 16, true); // fmt chunk size
  view.setUint16(20, 1, true); // PCM
  view.setUint16(22, 1, true); // mono
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true); // byte rate (sampleRate * blockAlign)
  view.setUint16(32, 2, true); // block align (channels * bytesPerSample)
  view.setUint16(34, 16, true); // bits per sample
  writeString(36, "data");
  view.setUint32(40, samples.length * 2, true);

  let offset = 44;
  for (let i = 0; i < samples.length; i++) {
    const s = Math.max(-1, Math.min(1, samples[i]));
    view.setInt16(offset, s < 0 ? s * 0x8000 : s * 0x7fff, true);
    offset += 2;
  }
  return buffer;
}

// arrayBufferToBase64 base64-encodes bytes in chunks to avoid call-stack limits.
function arrayBufferToBase64(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let binary = "";
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(binary);
}
