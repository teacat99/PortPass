// Helpers shared by every UI surface that renders a preset-category
// icon. Categories carry a single `icon` string that holds either an
// emoji glyph (e.g. "🔐") or an http(s):// image URL pointing to a
// favicon-style asset (favicon.im, Google's favicon API, etc.). The
// frontend decides how to render based on this prefix instead of a
// dedicated `kind` column to keep the data model simple.

export function isImageIcon(icon: string | null | undefined): boolean {
  if (!icon) return false
  return /^https?:\/\//i.test(icon.trim())
}

// Curated emoji set surfaced in the picker panel. Hand-picked so the
// whole grid renders consistently across the major emoji fonts (no
// platform-specific glyphs that turn into tofu on older Android).
export const ICON_EMOJI_PRESETS: string[] = [
  '🔐', '🌐', '🗄️', '📬', '🎮', '🔌',
  '🛠️', '📡', '🎬', '🎨', '💾', '🚀',
  '🔋', '⚙️', '🛡️', '🧪', '📦', '🎯',
  '🌀', '🧊', '⚡', '🔭', '🪪', '🪟',
  '🧱', '🧰', '🔔', '☁️', '🧭', '📊'
]
