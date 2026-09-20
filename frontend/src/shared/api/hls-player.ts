export class HlsPlaybackError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'HlsPlaybackError'
  }
}

export class AuthenticatedHlsPlayer {
  async attach(element: HTMLVideoElement, manifestUrl: string): Promise<() => void> {
    const token = sessionStorage.getItem('beast.access-token')
    const normalizedURL = new URL(manifestUrl, window.location.origin)
    if (normalizedURL.origin !== window.location.origin) throw new HlsPlaybackError('HLS manifestの参照先が不正です')
    const { default: Hls } = await import('hls.js')
    if (Hls.isSupported()) {
      const hls = new Hls({
        xhrSetup: (request, url) => {
          const sameOrigin = new URL(url, window.location.origin).origin === window.location.origin
          request.withCredentials = sameOrigin
          if (token && sameOrigin) request.setRequestHeader('Authorization', `Bearer ${token}`)
        },
      })
      const ready = new Promise<void>((resolve, reject) => {
        const onManifest = (_event: string, data: { levels?: Array<{ videoCodec?: string; audioCodec?: string }> }) => {
          void verifyMediaCapabilities(data.levels ?? []).then(() => { cleanup(); resolve() }).catch((error) => { cleanup(); reject(error) })
        }
        const onFatal = (_event: string, data: { fatal?: boolean; details?: string }) => { if (data.fatal) { cleanup(); reject(new HlsPlaybackError(data.details ?? 'HLSの読み込みに失敗しました')) } }
        const cleanup = () => { hls.off(Hls.Events.MANIFEST_PARSED, onManifest); hls.off(Hls.Events.ERROR, onFatal) }
        hls.on(Hls.Events.MANIFEST_PARSED, onManifest)
        hls.on(Hls.Events.ERROR, onFatal)
      })
      hls.loadSource(normalizedURL.toString())
      hls.attachMedia(element)
      await ready.catch((error) => {
        hls.destroy()
        throw error
      })
      return () => {
        hls.destroy()
        element.removeAttribute('src')
        element.load()
      }
    }
    if (!element.canPlayType('application/vnd.apple.mpegurl')) throw new HlsPlaybackError('このブラウザはHLS再生に対応していません')
    element.src = normalizedURL.toString()
    return () => {
      element.removeAttribute('src')
      element.load()
    }
  }
}

async function verifyMediaCapabilities(levels: Array<{ videoCodec?: string; audioCodec?: string }>) {
  if (!('mediaCapabilities' in navigator) || typeof navigator.mediaCapabilities.decodingInfo !== 'function') return
  const codecs = [...new Set(levels.flatMap((level) => [level.videoCodec ? { kind: 'video', codec: level.videoCodec } : null, level.audioCodec ? { kind: 'audio', codec: level.audioCodec } : null].filter((value): value is { kind: string; codec: string } => value !== null)))]
  for (const { kind, codec } of codecs) {
    const configuration = kind === 'video' ? { type: 'media-source' as const, video: { contentType: `video/mp4; codecs="${codec}"`, width: 1920, height: 1080, bitrate: 8_000_000, framerate: 30 } } : { type: 'media-source' as const, audio: { contentType: `audio/mp4; codecs="${codec}"`, channels: '2', bitrate: 256_000, samplerate: 48_000 } }
    const result = await navigator.mediaCapabilities.decodingInfo(configuration)
    if (!result.supported) throw new HlsPlaybackError(`端末がcodec ${codec}を再生できません`)
  }
}
