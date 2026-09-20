import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { GraphQlVideoApi, VideoApiError } from '../../../shared/api/video'
import type { RecommendationKind, VideoApi, VideoRecord } from '../../../shared/api/video'
import { ChevronRightIcon, CloseIcon, HomeIcon, LibraryIcon, LockIcon, MoreIcon, PlayIcon, SearchIcon, StarIcon, UploadIcon } from '../../../shared/ui/icons'

const previewVideoUrl = 'https://storage.googleapis.com/gtv-videos-bucket/sample/ForBiggerEscapes.mp4'

const demoVideos: VideoRecord[] = [
  { id: 'v-01', durationSeconds: 623, uploadedAt: '2026-09-18', lastPlayedAt: '2026-09-14', playCount: 4, rating: 5, tags: ['旅', '夕方'], encryptedTags: 'demo', thumbnailUrl: '', playbackUrl: previewVideoUrl, progress: 'ready', progressRatio: 1, metadata: { algorithm: 'AES-GCM', chunkSize: 1048576, keyVersion: 'v2', nonce: 'demo', encryptedDataKey: 'demo', sharedKeyId: 'demo' } },
  { id: 'v-02', durationSeconds: 218, uploadedAt: '2026-09-17', lastPlayedAt: null, playCount: 0, rating: null, tags: ['散歩', '街'], encryptedTags: 'demo', thumbnailUrl: '', playbackUrl: previewVideoUrl, progress: 'ready', progressRatio: 1, metadata: { algorithm: 'AES-GCM', chunkSize: 1048576, keyVersion: 'v2', nonce: 'demo', encryptedDataKey: 'demo', sharedKeyId: 'demo' } },
  { id: 'v-03', durationSeconds: 496, uploadedAt: '2026-09-12', lastPlayedAt: '2026-09-16', playCount: 9, rating: 4, tags: ['料理', '週末'], encryptedTags: 'demo', thumbnailUrl: '', playbackUrl: previewVideoUrl, progress: 'ready', progressRatio: 1, metadata: { algorithm: 'AES-GCM', chunkSize: 1048576, keyVersion: 'v2', nonce: 'demo', encryptedDataKey: 'demo', sharedKeyId: 'demo' } },
  { id: 'v-04', durationSeconds: 1280, uploadedAt: '2026-09-11', lastPlayedAt: null, playCount: 0, rating: null, tags: ['旅行'], encryptedTags: 'demo', thumbnailUrl: '', playbackUrl: '', progress: 'encoding', progressRatio: .4, metadata: { algorithm: 'AES-GCM', chunkSize: 1048576, keyVersion: 'v3', nonce: 'demo', encryptedDataKey: 'demo', sharedKeyId: 'demo' } },
  { id: 'v-05', durationSeconds: 92, uploadedAt: '2026-09-08', lastPlayedAt: '2026-09-15', playCount: 2, rating: 3, tags: ['猫', '日常'], encryptedTags: 'demo', thumbnailUrl: '', playbackUrl: previewVideoUrl, progress: 'ready', progressRatio: 1, metadata: { algorithm: 'AES-GCM', chunkSize: 1048576, keyVersion: 'v2', nonce: 'demo', encryptedDataKey: 'demo', sharedKeyId: 'demo' } },
]

type Tab = 'for-you' | 'unwatched' | 'favorites'
const recommendationKinds = ['FAVORITES', 'RECENTLY_UNPLAYED_FAVORITES', 'UNWATCHED'] as const

export function LibraryPage() {
  const demoMode = import.meta.env.VITE_DEMO_MODE === 'true'
  const [hasSession] = useState(() => demoMode || Boolean(sessionStorage.getItem('beast.access-token')))
  const api = useMemo<VideoApi>(() => new GraphQlVideoApi(), [])
  const [videos, setVideos] = useState<VideoRecord[]>(demoMode ? demoVideos : [])
  const [recommendations, setRecommendations] = useState<Record<RecommendationKind, VideoRecord[]>>({ FAVORITES: [], RECENTLY_UNPLAYED_FAVORITES: [], UNWATCHED: [] })
  const [isLoading, setIsLoading] = useState(!demoMode)
  const [error, setError] = useState<string | null>(null)
  const [tab, setTab] = useState<Tab>('for-you')
  const [query, setQuery] = useState('')
  const [tag, setTag] = useState<string | null>(null)
  const [selectedVideo, setSelectedVideo] = useState<VideoRecord | null>(null)
  const [editingVideo, setEditingVideo] = useState<VideoRecord | null>(null)
  const [tagDraft, setTagDraft] = useState('')
  const [isPlaying, setIsPlaying] = useState(false)

  useEffect(() => {
    if (demoMode || !hasSession) return
    let active = true
    async function loadLibrary() {
      setIsLoading(true)
      try {
        const listedVideos = await api.listVideos()
        const recommendationResults = await Promise.allSettled(recommendationKinds.map((kind) => api.recommendations(kind)))
        if (!active) return
        setVideos(listedVideos)
        const nextRecommendations = { FAVORITES: [], RECENTLY_UNPLAYED_FAVORITES: [], UNWATCHED: [] } as Record<RecommendationKind, VideoRecord[]>
        recommendationResults.forEach((result, index) => { if (result.status === 'fulfilled') nextRecommendations[recommendationKinds[index]] = result.value })
        setRecommendations(nextRecommendations)
        const failedRecommendation = recommendationResults.find((result): result is PromiseRejectedResult => result.status === 'rejected')
        if (failedRecommendation) setError(errorMessage(failedRecommendation.reason))
      } catch (reason) {
        if (active) setError(errorMessage(reason))
      } finally {
        if (active) setIsLoading(false)
      }
    }
    void loadLibrary()
    return () => { active = false }
  }, [api, demoMode, hasSession])

  const allTags = useMemo(() => [...new Set(videos.flatMap((video) => video.tags))], [videos])
  const filteredVideos = useMemo(() => videos.filter((video) => {
    const matchesQuery = query.trim() === '' || video.tags.some((videoTag) => videoTag.includes(query.trim()))
    const matchesTag = tag === null || video.tags.includes(tag)
    const matchesTab = tab === 'for-you' || (tab === 'unwatched' ? video.playCount === 0 : video.rating !== null && video.rating >= 4)
    return matchesQuery && matchesTag && matchesTab
  }), [query, tab, tag, videos])
  const favorites = videos.filter((video) => video.rating !== null && video.rating >= 4)
  const unwatched = videos.filter((video) => video.playCount === 0)

  const favoriteRail = recommendations.FAVORITES.length ? recommendations.FAVORITES : favorites
  const unwatchedRail = recommendations.UNWATCHED.length ? recommendations.UNWATCHED : unwatched

  function openVideo(video: VideoRecord) {
    if (video.progress !== 'ready' || !video.playbackUrl) {
      setError('この動画はまだ端末で再生できません')
      return
    }
    setSelectedVideo(video)
    setIsPlaying(true)
    if (demoMode) {
      setVideos((current) => current.map((item) => item.id === video.id ? { ...item, playCount: item.playCount + 1, lastPlayedAt: new Date().toISOString() } : item))
      return
    }
    void api.recordPlayback(video.id).then((updated) => { setVideos((current) => replaceVideo(current, updated)); setSelectedVideo(updated) }).catch((reason) => setError(errorMessage(reason)))
  }

  function rateVideo(rating: number) {
    if (!selectedVideo) return
    const nextRating = selectedVideo.rating === rating ? null : rating
    if (demoMode) {
      setVideos((current) => current.map((item) => item.id === selectedVideo.id ? { ...item, rating: nextRating } : item))
      setSelectedVideo((current) => current ? { ...current, rating: nextRating } : current)
      return
    }
    void api.rateVideo(selectedVideo.id, nextRating).then((updated) => { setVideos((current) => replaceVideo(current, updated)); setSelectedVideo(updated) }).catch((reason) => setError(errorMessage(reason)))
  }

  function saveTags(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!editingVideo) return
    if (!demoMode) { setError('タグ更新APIはまだ提供されていません'); setEditingVideo(null); return }
    const tags = tagDraft.split(/\s+/).map((item) => item.trim()).filter(Boolean).slice(0, 8)
    setVideos((current) => current.map((item) => item.id === editingVideo.id ? { ...item, tags: [...new Set(tags)] } : item))
    setEditingVideo(null)
  }

  if (!hasSession) return <UnauthenticatedScreen />

  return (
    <div className="app-shell">
      <header className="topbar">
        <a className="wordmark" href="/" aria-label="beast ホーム"><span className="wordmark-mark">b</span><span>beast</span></a>
        <div className="topbar-actions"><span className="secure-label"><LockIcon size={13} /> encrypted library</span><button className="avatar" aria-label="アカウント">Y</button></div>
      </header>

      <main>
        {error && <div className="api-notice" role="alert"><span>{error}</span><button onClick={() => window.location.reload()}>再試行</button></div>}
        {isLoading && <div className="loading-state" role="status">ライブラリを読み込んでいます…</div>}
        <section className="intro-section">
          <p className="eyebrow">YOUR PRIVATE LIBRARY <span>·</span> 2026.09.20</p>
          <h1>今夜は、どの記憶を<br /><span>再生しますか？</span></h1>
          <p className="intro-copy">お気に入りと、まだ見ていない動画を<br />今の気分に合わせて並べました。</p>
          <label className="search-field"><SearchIcon size={18} /><span className="visually-hidden">タグで検索</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="タグで探す" /></label>
        </section>

        <nav className="tab-nav" aria-label="動画の絞り込み">
          {([['for-you', 'あなたへ'], ['unwatched', '未視聴'], ['favorites', 'お気に入り']] as const).map(([value, label]) => <button key={value} className={tab === value ? 'tab-button is-active' : 'tab-button'} onClick={() => setTab(value)}>{label}</button>)}
        </nav>
        <div className="tag-row" aria-label="タグで絞り込む">
          <button className={tag === null ? 'tag-chip is-active' : 'tag-chip'} onClick={() => setTag(null)}>すべて</button>
          {allTags.map((item) => <button key={item} className={tag === item ? 'tag-chip is-active' : 'tag-chip'} onClick={() => setTag(tag === item ? null : item)}>#{item}</button>)}
          <button className="tag-chip tag-chip-add" onClick={() => { setEditingVideo(videos[0]); setTagDraft(videos[0]?.tags.join(' ') ?? '') }}><UploadIcon size={13} />編集</button>
        </div>

        <section className="featured-section" aria-labelledby="featured-heading">
          <div className="section-heading"><div><p className="section-kicker">01 / PICKED FOR YOU</p><h2 id="featured-heading">今日の一本</h2></div><span className="section-count">{filteredVideos.length}本</span></div>
          {filteredVideos[0] ? <FeaturedCard video={filteredVideos[0]} onOpen={() => openVideo(filteredVideos[0])} /> : <EmptyState />}
        </section>

        <Rail title="あなたのお気に入り" kicker="02 / YOUR FAVOURITES" videos={favoriteRail} onOpen={openVideo} onEdit={(video) => { setEditingVideo(video); setTagDraft(video.tags.join(' ')) }} />
        <Rail title="最近再生していないお気に入り" kicker="03 / RETURN TO FAVOURITES" videos={recommendations.RECENTLY_UNPLAYED_FAVORITES} onOpen={openVideo} onEdit={(video) => { setEditingVideo(video); setTagDraft(video.tags.join(' ')) }} />
        <Rail title="まだ再生していない動画" kicker="04 / WAITING FOR YOU" videos={unwatchedRail} onOpen={openVideo} onEdit={(video) => { setEditingVideo(video); setTagDraft(video.tags.join(' ')) }} />

        <section className="all-section" aria-labelledby="all-heading"><div className="section-heading"><div><p className="section-kicker">04 / THE ARCHIVE</p><h2 id="all-heading">すべての動画</h2></div><span className="section-count">{filteredVideos.length}本</span></div><div className="video-grid">{filteredVideos.map((video) => <VideoCard key={video.id} video={video} onOpen={() => openVideo(video)} onEdit={() => { setEditingVideo(video); setTagDraft(video.tags.join(' ')) }} />)}</div></section>
      </main>

      <footer className="footer"><p>記憶は、あなたの手元に。</p><div><span>beast · encrypted by default</span><span>© 2026</span></div></footer>
      <div className="bottom-nav"><button className="bottom-nav-item is-active"><HomeIcon size={19} /><span>ホーム</span></button><button className="bottom-nav-item"><LibraryIcon size={19} /><span>ライブラリ</span></button><button className="bottom-nav-item"><UploadIcon size={19} /><span>アップロード</span></button></div>

      {selectedVideo && <PlayerDialog video={selectedVideo} isPlaying={isPlaying} onPlayingChange={setIsPlaying} onClose={() => { setSelectedVideo(null); setIsPlaying(false) }} onRate={rateVideo} />}
      {editingVideo && <TagDialog video={editingVideo} draft={tagDraft} onDraftChange={setTagDraft} onClose={() => setEditingVideo(null)} onSave={saveTags} />}
    </div>
  )
}

function FeaturedCard({ video, onOpen }: { video: VideoRecord; onOpen: () => void }) {
  return <button className="featured-card thumb--amber" onClick={onOpen}><div className="featured-overlay"><span className="encrypted-badge"><LockIcon size={12} />暗号化済み</span><strong>{displayTags(video)}</strong><span>{video.playCount}回再生  ·  {formatDuration(video.durationSeconds)}</span></div><span className="play-button"><PlayIcon size={22} /></span><span className="playbook-mark">01</span></button>
}

function Rail({ title, kicker, videos, onOpen, onEdit }: { title: string; kicker: string; videos: VideoRecord[]; onOpen: (video: VideoRecord) => void; onEdit: (video: VideoRecord) => void }) {
  if (videos.length === 0) return null
  return <section className="rail-section"><div className="section-heading"><div><p className="section-kicker">{kicker}</p><h2>{title}</h2></div><button className="see-all">すべて見る <ChevronRightIcon size={15} /></button></div><div className="rail-track">{videos.map((video) => <VideoCard key={video.id} video={video} onOpen={() => onOpen(video)} onEdit={() => onEdit(video)} />)}</div></section>
}

function VideoCard({ video, onOpen, onEdit }: { video: VideoRecord; onOpen: () => void; onEdit: () => void }) {
  const color = `thumb--${['amber', 'violet', 'moss', 'rose', 'blue'][Number(video.id.slice(-1)) % 5]}`
  return <article className="video-card"><button className={`video-thumb ${color}`} onClick={onOpen} disabled={video.progress !== 'ready'}><span className="thumb-play"><PlayIcon size={16} /></span><span className="thumb-duration">{video.progress === 'encoding' ? '準備中' : formatDuration(video.durationSeconds)}</span>{video.rating && <span className="thumb-rating"><StarIcon size={12} filled /> {video.rating}</span>}</button><div className="video-card-meta"><div><strong>{displayTags(video)}</strong><span>{video.playCount}回再生  ·  {formatDuration(video.durationSeconds)}</span></div><button className="more-button" onClick={onEdit} aria-label="タグを編集"><MoreIcon size={17} /></button></div></article>
}

function PlayerDialog({ video, isPlaying, onPlayingChange, onClose, onRate }: { video: VideoRecord; isPlaying: boolean; onPlayingChange: (playing: boolean) => void; onClose: () => void; onRate: (rating: number) => void }) {
  return <div className="player-backdrop" role="dialog" aria-modal="true" aria-label="動画プレーヤー"><div className="player-panel"><div className="player-top"><button className="icon-button" onClick={onClose} aria-label="閉じる"><CloseIcon /></button><span><LockIcon size={13} /> クライアント側で復号</span></div>{video.playbackUrl ? <video className="player-video" src={video.playbackUrl} controls autoPlay={isPlaying} onPlay={() => onPlayingChange(true)} onPause={() => onPlayingChange(false)} /> : <div className="player-unavailable">復号済みの再生URLがありません。</div>}<div className="player-meta"><div><span className="player-tags">{video.tags.length ? video.tags.map((tag) => `#${tag}`).join('  ') : '暗号化タグ（端末鍵が必要です）'}</span><p>{video.playCount}回再生  ·  {formatDuration(video.durationSeconds)}</p></div><div className="rating-row" aria-label="星評価">{[1, 2, 3, 4, 5].map((rating) => <button key={rating} onClick={() => onRate(rating)} aria-label={`${rating}つ星`}><StarIcon size={22} filled={(video.rating ?? 0) >= rating} /></button>)}</div></div><p className="gesture-hint">ダブルタップで10秒移動 · 長押しで1.75倍速</p></div></div>
}

function TagDialog({ video, draft, onDraftChange, onClose, onSave }: { video: VideoRecord; draft: string; onDraftChange: (value: string) => void; onClose: () => void; onSave: (event: FormEvent<HTMLFormElement>) => void }) {
  return <div className="modal-backdrop"><form className="modal-card" onSubmit={onSave}><div className="modal-heading"><div><p className="section-kicker">VIDEO TAGS</p><h2>タグを編集</h2></div><button className="icon-button" type="button" onClick={onClose} aria-label="閉じる"><CloseIcon /></button></div><label className="field-label">タグ<span>スペース区切り</span><input value={draft} onChange={(event) => onDraftChange(event.target.value)} autoFocus /></label><div className="modal-actions"><button type="button" className="button-secondary" onClick={onClose}>キャンセル</button><button type="submit" className="button-primary">保存する</button></div><p className="modal-note"><LockIcon size={13} /> タグも暗号化して保存されます · {video.metadata.algorithm} / key v{video.metadata.keyVersion}</p></form></div>
}

function EmptyState() { return <div className="empty-state"><SearchIcon size={22} /><p>条件に合う動画がありません。</p><span>タグやタブを変えて探してみてください。</span></div> }
function UnauthenticatedScreen() { return <div className="auth-required"><div className="auth-required-mark"><LockIcon size={25} /></div><p className="eyebrow">PRIVATE LIBRARY</p><h1>ログインして<br /><span>動画を再生します。</span></h1><p>このライブラリは所有者だけがアクセスできます。ログイン後にもう一度開いてください。</p><button className="button-primary" onClick={() => window.location.assign('/login')}>ログイン</button></div> }
function displayTags(video: VideoRecord) { return video.tags.length ? video.tags.join('  ·  ') : '暗号化タグ（復号待ち）' }
function replaceVideo(videos: VideoRecord[], updated: VideoRecord) { return videos.map((video) => video.id === updated.id ? updated : video) }
function errorMessage(reason: unknown) { return reason instanceof VideoApiError ? reason.message : '動画ライブラリの取得に失敗しました' }
function formatDuration(seconds: number) { const minutes = Math.floor(seconds / 60); return `${minutes}:${String(seconds % 60).padStart(2, '0')}` }
