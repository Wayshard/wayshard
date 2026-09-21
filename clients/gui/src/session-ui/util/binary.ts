// Adapted binary-search helper retained from the imported OpenCode client
// foundation, rewritten for Wayshard-owned client source.
export interface SearchResult<T> { found: boolean; index: number; item: T | undefined }
export const Binary = {
  search<T>(list: readonly T[], target: string, key: (item: T) => string): SearchResult<T> {
    let lo = 0, hi = list.length - 1
    while (lo <= hi) {
      const mid = (lo + hi) >> 1
      const value = key(list[mid])
      if (value === target) return { found: true, index: mid, item: list[mid] }
      if (value < target) lo = mid + 1
      else hi = mid - 1
    }
    return { found: false, index: lo, item: undefined }
  },
}
