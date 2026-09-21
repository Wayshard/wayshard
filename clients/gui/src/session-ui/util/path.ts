// Adapted path helpers retained from the imported OpenCode client foundation,
// rewritten for Wayshard-owned client source.
export function getFilename(path: string | undefined): string {
  if (!path) return ""
  const n = path.replace(/\\/g, "/"); const i = n.lastIndexOf("/"); return i >= 0 ? n.slice(i + 1) : n
}
export function getDirectory(path: string | undefined): string {
  if (!path) return ""
  const n = path.replace(/\\/g, "/"); const i = n.lastIndexOf("/"); return i > 0 ? n.slice(0, i) : ""
}
export function getFilenameTruncated(path: string | undefined, max = 40): string {
  const name = getFilename(path)
  if (name.length <= max) return name
  const ext = name.includes(".") ? name.slice(name.lastIndexOf(".")) : ""
  return name.slice(0, Math.max(1, max - ext.length - 1)) + "…" + ext
}
