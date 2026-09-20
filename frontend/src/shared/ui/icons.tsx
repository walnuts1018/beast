import type { ReactNode } from 'react'

type IconProps = { size?: number; strokeWidth?: number }

const icon = (paths: ReactNode, { size = 20, strokeWidth = 1.8 }: IconProps = {}) => (
  <svg aria-hidden="true" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
    {paths}
  </svg>
)

export const PlayIcon = (props: IconProps) => icon(<path fill="currentColor" stroke="none" d="m8 5 11 7-11 7V5Z" />, props)
export const PauseIcon = (props: IconProps) => icon(<><path d="M8 5v14M16 5v14" /></>, props)
export const SearchIcon = (props: IconProps) => icon(<><circle cx="10.8" cy="10.8" r="6.8" /><path d="m16 16 4.4 4.4" /></>, props)
export const LockIcon = (props: IconProps) => icon(<><rect x="5" y="10" width="14" height="10" rx="2" /><path d="M8 10V7a4 4 0 0 1 8 0v3" /></>, props)
export const MoreIcon = (props: IconProps) => icon(<><circle cx="5" cy="12" r="1" fill="currentColor" /><circle cx="12" cy="12" r="1" fill="currentColor" /><circle cx="19" cy="12" r="1" fill="currentColor" /></>, props)
export const HeartIcon = ({ filled = false, ...props }: IconProps & { filled?: boolean }) => icon(<path fill={filled ? 'currentColor' : 'none'} d="M20.8 8.8c0 5.1-8.8 10-8.8 10s-8.8-4.9-8.8-10A4.7 4.7 0 0 1 12 6a4.7 4.7 0 0 1 8.8 2.8Z" />, props)
export const StarIcon = ({ filled = false, ...props }: IconProps & { filled?: boolean }) => icon(<path fill={filled ? 'currentColor' : 'none'} d="m12 3 2.7 5.5 6.1.9-4.4 4.3 1 6.1-5.4-2.9-5.4 2.9 1-6.1-4.4-4.3 6.1-.9L12 3Z" />, props)
export const ClockIcon = (props: IconProps) => icon(<><circle cx="12" cy="12" r="8.5" /><path d="M12 7v5l3.2 2" /></>, props)
export const ChevronRightIcon = (props: IconProps) => icon(<path d="m9 5 7 7-7 7" />, props)
export const ChevronDownIcon = (props: IconProps) => icon(<path d="m6 9 6 6 6-6" />, props)
export const SlidersIcon = (props: IconProps) => icon(<><path d="M4 7h16M4 17h16" /><circle cx="9" cy="7" r="2" fill="var(--color-paper)" /><circle cx="15" cy="17" r="2" fill="var(--color-paper)" /></>, props)
export const HomeIcon = (props: IconProps) => icon(<><path d="m4 10 8-6 8 6v9a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1v-9Z" /><path d="M9 20v-6h6v6" /></>, props)
export const LibraryIcon = (props: IconProps) => icon(<><rect x="4" y="4" width="6" height="16" rx="1" /><rect x="14" y="4" width="6" height="16" rx="1" /></>, props)
export const UploadIcon = (props: IconProps) => icon(<><path d="M12 16V4m0 0L7 9m5-5 5 5" /><path d="M5 14v5h14v-5" /></>, props)
export const CloseIcon = (props: IconProps) => icon(<><path d="m6 6 12 12M18 6 6 18" /></>, props)
export const VolumeIcon = (props: IconProps) => icon(<><path d="M4 10v4h4l5 4V6L8 10H4Z" /><path d="M16 9.5a4 4 0 0 1 0 5" /></>, props)
export const FullscreenIcon = (props: IconProps) => icon(<><path d="M8 4H4v4M16 4h4v4M8 20H4v-4M20 16v4h-4" /></>, props)
