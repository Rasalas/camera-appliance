import {
  House, MonitorPlay, Camera, Upload, Server, SlidersHorizontal, ShieldCheck,
  Network, UserRound, Wrench, Activity, History, RefreshCw, Headset, Logs, Info,
  Plus, Pencil, Mail, Download, ExternalLink, Coffee, Heart, Search, EllipsisVertical, X, ChevronRight
} from '@lucide/vue'

// Import individual library components so unused icons stay out of the bundle.
export const icons = {
  home: House, live: MonitorPlay, camera: Camera, upload: Upload, system: Server,
  settings: SlidersHorizontal, shield: ShieldCheck, relay: Network, identity: UserRound,
  tools: Wrench, activity: Activity, backup: History, update: RefreshCw, support: Headset,
  log: Logs, info: Info, plus: Plus, edit: Pencil, mail: Mail, download: Download,
  external: ExternalLink, coffee: Coffee, heart: Heart, search: Search,
  overflow: EllipsisVertical, close: X, chevron: ChevronRight
} as const
export type IconName = keyof typeof icons
