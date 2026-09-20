export type VideoProgress = 'uploaded' | 'encoding' | 'ready' | 'failed'

export type EncryptedMetadata = {
  algorithm: string
  chunkSize: number
  keyVersion: string
  nonce: string
  encryptedDataKey: string
  sharedKeyId: string
}

export type VideoRecord = {
  id: string
  durationSeconds: number
  uploadedAt: string
  lastPlayedAt: string | null
  playCount: number
  rating: number | null
  tags: string[]
  encryptedTags: string
  thumbnailUrl: string
  playback: 'encrypted-dash' | 'demo'
  dashManifestUrl: string
  playbackUrl: string
  progress: VideoProgress
  progressRatio: number
  metadata: EncryptedMetadata
}

export type RecommendationKind = 'FAVORITES' | 'RECENTLY_UNPLAYED_FAVORITES' | 'UNWATCHED'

export type ClientKeyEnvelope = {
  algorithm: 'ECDH-P256'
  publicKey: string
  wrappedKey: string
  keyVersion: number
}

export type VideoApi = {
  listVideos: () => Promise<VideoRecord[]>
  recommendations: (kind: RecommendationKind) => Promise<VideoRecord[]>
  rateVideo: (videoId: string, rating: number | null) => Promise<VideoRecord>
  recordPlayback: (videoId: string) => Promise<VideoRecord>
  fetchDashArtifact: (videoId: string, artifactPath?: string, signal?: AbortSignal) => Promise<DashArtifact>
  createClientKeyEnvelope: (publicKey: string) => Promise<ClientKeyEnvelope>
}

export type DashArtifact = {
  path: string
  bytes: ArrayBuffer
  contentType: string
  encryption: EncryptedMetadata
}

type ApiVideo = {
  id: string
  status: 'UPLOADED' | 'ENCODING' | 'READY' | 'FAILED'
  encryptedTags: string
  playCount: number
  rating: number | null
  lastPlayedAt: string | null
  progress: number
  encryption: { algorithm: string; chunkSize: number; keyVersion: string; nonce: string; encryptedDataKey: string; sharedKeyID: string }
}

type GraphQlPayload<T> = { data?: T; errors?: Array<{ message: string }> }
type TagsDecoder = (video: ApiVideo) => Promise<string[]>

export class VideoApiError extends Error {
  constructor(message: string, readonly status?: number) {
    super(message)
    this.name = 'VideoApiError'
  }
}

const videoSelection = `
  id status encryptedTags playCount rating lastPlayedAt progress
  encryption { algorithm chunkSize keyVersion nonce encryptedDataKey sharedKeyID }
`

export class GraphQlVideoApi implements VideoApi {
  constructor(
    private readonly endpoint = '/graphql/query',
    private readonly getAccessToken: () => string | null = () => sessionStorage.getItem('beast.access-token'),
    private readonly decodeTags: TagsDecoder = async () => [],
  ) {}

  async listVideos() {
    const payload = await this.request<{ videos: ApiVideo[] }>(`query ListVideos { videos { ${videoSelection} } }`)
    return Promise.all((payload.videos ?? []).map((video) => this.toVideoRecord(video)))
  }

  async recommendations(kind: RecommendationKind) {
    const payload = await this.request<{ recommendations: ApiVideo[] }>(`query Recommendations(${'$'}kind: RecommendationKind!) { recommendations(kind: ${'$'}kind) { ${videoSelection} } }`, { kind })
    return Promise.all((payload.recommendations ?? []).map((video) => this.toVideoRecord(video)))
  }

  async rateVideo(videoId: string, rating: number | null) {
    const payload = await this.request<{ rateVideo: ApiVideo }>(`mutation RateVideo(${'$'}id: ID!, ${'$'}rating: Int) { rateVideo(id: ${'$'}id, rating: ${'$'}rating) { ${videoSelection} } }`, { id: videoId, rating })
    return this.toVideoRecord(payload.rateVideo)
  }

  async recordPlayback(videoId: string) {
    const payload = await this.request<{ recordPlayback: ApiVideo }>(`mutation RecordPlayback(${'$'}id: ID!) { recordPlayback(id: ${'$'}id) { ${videoSelection} } }`, { id: videoId })
    return this.toVideoRecord(payload.recordPlayback)
  }

  async fetchDashArtifact(videoId: string, artifactPath = 'manifest.mpd', signal?: AbortSignal): Promise<DashArtifact> {
    const token = this.getAccessToken()
    if (!token) throw new VideoApiError('ログインセッションがありません')
    const encodedPath = artifactPath.split('/').filter(Boolean).map((part) => encodeURIComponent(part)).join('/')
    const url = resolveMediaUrl(`/api/videos/${encodeURIComponent(videoId)}/dash/${encodedPath}`, this.endpoint)
    if (!url) throw new VideoApiError('DASH artifactのURLを解決できませんでした')
    let response: Response
    try {
      response = await fetch(url, { headers: { Accept: 'application/octet-stream', Authorization: `Bearer ${token}` }, signal })
    } catch {
      throw new VideoApiError('DASH artifactを取得できませんでした')
    }
    if (!response.ok) throw new VideoApiError(`DASH artifactの取得に失敗しました (${response.status})`, response.status)
    const encryption = encryptionMetadataFromHeaders(response.headers)
    return { path: artifactPath, bytes: await response.arrayBuffer(), contentType: response.headers.get('content-type') ?? 'application/octet-stream', encryption }
  }

  async createClientKeyEnvelope(_publicKey: string): Promise<ClientKeyEnvelope> {
    throw new VideoApiError('クライアント鍵登録APIはまだ利用できません')
  }

  private async request<T>(query: string, variables: Record<string, unknown> = {}): Promise<T> {
    const token = this.getAccessToken()
    if (!token) throw new VideoApiError('ログインセッションがありません')
    let response: Response
    try {
      response = await fetch(this.endpoint, {
        method: 'POST',
        headers: { Accept: 'application/json', Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
        body: JSON.stringify({ query, variables }),
      })
    } catch {
      throw new VideoApiError('APIに接続できませんでした')
    }
    let payload: GraphQlPayload<T>
    try {
      payload = await response.json() as GraphQlPayload<T>
    } catch {
      throw new VideoApiError('APIの応答を読み取れませんでした', response.status)
    }
    if (!response.ok) throw new VideoApiError(`APIエラー (${response.status})`, response.status)
    if (payload.errors?.length) throw new VideoApiError(payload.errors.map((error) => error.message).join('、'))
    if (!payload.data) throw new VideoApiError('APIからデータを取得できませんでした')
    return payload.data
  }

  private async toVideoRecord(video: ApiVideo): Promise<VideoRecord> {
    const tags = await this.decodeTags(video)
    return {
      id: video.id,
      durationSeconds: 0,
      uploadedAt: '',
      lastPlayedAt: video.lastPlayedAt,
      playCount: video.playCount,
      rating: video.rating,
      tags,
      encryptedTags: video.encryptedTags,
      thumbnailUrl: '',
      playback: 'encrypted-dash',
      dashManifestUrl: resolveMediaUrl(`/api/videos/${encodeURIComponent(video.id)}/dash/manifest.mpd`, this.endpoint),
      playbackUrl: '',
      progress: video.status.toLowerCase() as VideoProgress,
      progressRatio: video.progress,
      metadata: {
        algorithm: video.encryption.algorithm,
        chunkSize: video.encryption.chunkSize,
        keyVersion: video.encryption.keyVersion,
        nonce: video.encryption.nonce,
        encryptedDataKey: video.encryption.encryptedDataKey,
        sharedKeyId: video.encryption.sharedKeyID,
      },
    }
  }
}

function encryptionMetadataFromHeaders(headers: Headers): EncryptedMetadata {
  const algorithm = headers.get('x-encryption-algorithm')
  const chunkSize = Number(headers.get('x-encryption-chunk-size'))
  const keyVersion = headers.get('x-encryption-key-version')
  const nonce = headers.get('x-encryption-nonce')
  const encryptedDataKey = headers.get('x-encryption-data-key')
  const sharedKeyId = headers.get('x-encryption-shared-key-id')
  if (!algorithm || !Number.isSafeInteger(chunkSize) || chunkSize <= 0 || !keyVersion || !nonce || !encryptedDataKey || !sharedKeyId) {
    throw new VideoApiError('DASH artifactの暗号化メタデータが不足しています')
  }
  return { algorithm, chunkSize, keyVersion, nonce, encryptedDataKey, sharedKeyId }
}

function resolveMediaUrl(path: string, endpoint: string) {
  try {
    return new URL(path, new URL(endpoint, window.location.origin)).toString()
  } catch {
    return ''
  }
}
