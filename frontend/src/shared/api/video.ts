export type VideoProgress = 'uploaded' | 'encoding' | 'ready' | 'failed'

export type VideoRecord = {
  id: string
  durationSeconds: number
  uploadedAt: string
  lastPlayedAt: string | null
  playCount: number
  rating: number | null
  tags: string[]
  thumbnailUrl: string
  playback: 'hls' | 'demo'
  hlsManifestUrl: string
  playbackUrl: string
  progress: VideoProgress
  progressRatio: number
}

export type RecommendationKind = 'FAVORITES' | 'RECENTLY_UNPLAYED_FAVORITES' | 'UNWATCHED'

export type VideoApi = {
  checkSession: () => Promise<boolean>
  listVideos: () => Promise<VideoRecord[]>
  recommendations: (kind: RecommendationKind) => Promise<VideoRecord[]>
  uploadVideo: (file: File, tags: string[]) => Promise<{ id: string; status: VideoProgress; progress: number }>
  updateVideoTags: (videoId: string, tags: string[]) => Promise<VideoRecord>
  rateVideo: (videoId: string, rating: number | null) => Promise<VideoRecord>
  recordPlayback: (videoId: string) => Promise<VideoRecord>
}

type ApiVideo = {
  id: string
  status: 'UPLOADED' | 'ENCODING' | 'READY' | 'FAILED'
  tags: string[]
  playCount: number
  rating: number | null
  lastPlayedAt: string | null
  progress: number
}

type GraphQlPayload<T> = { data?: T; errors?: Array<{ message: string }> }
export class VideoApiError extends Error {
  constructor(message: string, readonly status?: number) {
    super(message)
    this.name = 'VideoApiError'
  }
}

const videoSelection = `
  id status tags playCount rating lastPlayedAt progress
`

export class GraphQlVideoApi implements VideoApi {
  constructor(
    private readonly endpoint = '/graphql/query',
    private readonly getAccessToken: () => string | null = () => sessionStorage.getItem('beast.access-token'),
  ) {}

  async listVideos() {
    const payload = await this.request<{ videos: ApiVideo[] }>(`query ListVideos { videos { ${videoSelection} } }`)
    return Promise.all((payload.videos ?? []).map((video) => this.toVideoRecord(video)))
  }

  async uploadVideo(file: File, tags: string[]) {
    const token = this.getAccessToken()
    const body = new FormData()
    body.append('file', file)
    body.append('tags', JSON.stringify(tags))
    const response = await fetch(resolveMediaUrl('/api/videos/upload', this.endpoint), { method: 'POST', body, credentials: 'include', headers: token ? { Authorization: `Bearer ${token}` } : undefined })
    if (!response.ok) throw new VideoApiError(`動画のアップロードに失敗しました (${response.status})`, response.status)
    return await response.json() as { id: string; status: VideoProgress; progress: number }
  }

  async checkSession() {
    const token = this.getAccessToken()
    const headers = token ? { Authorization: `Bearer ${token}` } : undefined
    try {
      const response = await fetch(resolveMediaUrl('/api/auth/session', this.endpoint), { credentials: 'include', headers })
      return response.ok
    } catch {
      return false
    }
  }

  async recommendations(kind: RecommendationKind) {
    const payload = await this.request<{ recommendations: ApiVideo[] }>(`query Recommendations(${'$'}kind: RecommendationKind!) { recommendations(kind: ${'$'}kind) { ${videoSelection} } }`, { kind })
    return Promise.all((payload.recommendations ?? []).map((video) => this.toVideoRecord(video)))
  }

  async updateVideoTags(videoId: string, tags: string[]) {
    const payload = await this.request<{ updateVideoTags: ApiVideo }>(`mutation UpdateVideoTags(${'$'}id: ID!, ${'$'}input: UpdateVideoTagsInput!) { updateVideoTags(id: ${'$'}id, input: ${'$'}input) { ${videoSelection} } }`, { id: videoId, input: { tags } })
    return this.toVideoRecord(payload.updateVideoTags)
  }

  async rateVideo(videoId: string, rating: number | null) {
    const payload = await this.request<{ rateVideo: ApiVideo }>(`mutation RateVideo(${'$'}id: ID!, ${'$'}rating: Int) { rateVideo(id: ${'$'}id, rating: ${'$'}rating) { ${videoSelection} } }`, { id: videoId, rating })
    return this.toVideoRecord(payload.rateVideo)
  }

  async recordPlayback(videoId: string) {
    const payload = await this.request<{ recordPlayback: ApiVideo }>(`mutation RecordPlayback(${'$'}id: ID!) { recordPlayback(id: ${'$'}id) { ${videoSelection} } }`, { id: videoId })
    return this.toVideoRecord(payload.recordPlayback)
  }

  private async request<T>(query: string, variables: Record<string, unknown> = {}): Promise<T> {
    const token = this.getAccessToken()
    let response: Response
    try {
      response = await fetch(this.endpoint, {
        method: 'POST',
        headers: { Accept: 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}), 'Content-Type': 'application/json' },
        credentials: 'include',
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
    return {
      id: video.id,
      durationSeconds: 0,
      uploadedAt: '',
      lastPlayedAt: video.lastPlayedAt,
      playCount: video.playCount,
      rating: video.rating,
      tags: video.tags,
      thumbnailUrl: '',
      playback: 'hls',
      hlsManifestUrl: resolveMediaUrl(`/api/videos/${encodeURIComponent(video.id)}/hls/manifest.m3u8`, this.endpoint),
      playbackUrl: '',
      progress: video.status.toLowerCase() as VideoProgress,
      progressRatio: video.progress,
    }
  }
}

function resolveMediaUrl(path: string, endpoint: string) {
  try {
    return new URL(path, new URL(endpoint, window.location.origin)).toString()
  } catch {
    return ''
  }
}
