// Adapted content-hashing helpers retained from the imported OpenCode client
// foundation, rewritten for Wayshard-owned client source.
function fnv1a(input: string): string {
  let h = 0x811c9dc5
  for (let i = 0; i < input.length; i++) { h ^= input.charCodeAt(i); h = Math.imul(h, 0x01000193) }
  return (h >>> 0).toString(16).padStart(8, "0")
}
export function checksum(input: string | undefined | null): string { return input == null ? "" : fnv1a(input) }
export function sampledChecksum(input: string, sample = 4096): string {
  if (input.length <= sample * 2) return fnv1a(input)
  return fnv1a(input.slice(0, sample) + input.slice(input.length - sample) + String(input.length))
}
