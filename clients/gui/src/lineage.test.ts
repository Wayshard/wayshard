// Lineage verification: makes it difficult to delete the adapted OpenCode
// client foundation and replace it with a tiny fresh UI while still claiming
// source-lineage conformance. Reads clients/lineage.manifest.json and asserts
// the live adapted subtrees still exist at substantial size.
import { describe, expect, test } from "bun:test"
import { readdirSync, readFileSync, statSync } from "node:fs"
import { join, resolve } from "node:path"

const clientsRoot = resolve(import.meta.dir, "..", "..")
const manifest = JSON.parse(readFileSync(join(clientsRoot, "lineage.manifest.json"), "utf8"))

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const p = join(dir, entry)
    const st = statSync(p)
    if (st.isDirectory()) walk(p, out)
    else out.push(p)
  }
  return out
}

function countLoc(files: string[]): number {
  let loc = 0
  for (const f of files) {
    if (!/\.(ts|tsx)$/.test(f)) continue
    loc += readFileSync(f, "utf8").split("\n").length
  }
  return loc
}

describe("client source lineage", () => {
  test("upstream provenance is recorded", () => {
    expect(manifest.upstream.tag).toBe("v1.18.31")
    expect(manifest.upstream.commit).toBe("014614d35b397775e5d397a490fc72368c894ec2")
    expect(manifest.upstream.role).toContain("provenance-only")
  })

  for (const subtree of manifest.subtrees) {
    test(`${subtree.upstream} -> ${subtree.destination} remains substantial`, () => {
      const dir = join(clientsRoot, subtree.destination)
      const files = statSync(dir).isDirectory() ? walk(dir) : [dir]
      expect(files.length).toBeGreaterThanOrEqual(subtree.minFiles)
      if (subtree.minLoc > 0) {
        expect(countLoc(files)).toBeGreaterThanOrEqual(subtree.minLoc)
      }
      for (const marker of subtree.markers) {
        expect(files.some((f) => f.endsWith(marker))).toBe(true)
      }
    })
  }

  test("live clients contain no OpenCode runtime import specifier", () => {
    for (const pkg of ["sdk", "ui", "gui", "web", "tui"]) {
      const files = walk(join(clientsRoot, pkg, "src")).filter((f) => /\.(ts|tsx|json)$/.test(f) && !/\.(test|stories)\./.test(f))
      for (const f of files) {
        const text = readFileSync(f, "utf8")
        expect(text.includes("@opencode-ai/")).toBe(false)
      }
    }
  })
})

describe("client source lineage — per-file ancestry", () => {
  test("manifest records concrete per-file ancestry with upstream blobs", () => {
    expect(Array.isArray(manifest.fileAncestry)).toBe(true)
    expect(manifest.fileAncestry.length).toBeGreaterThanOrEqual(12)
    const appFiles = manifest.fileAncestry.filter((f: any) => f.upstream.startsWith("packages/app/src/"))
    const tuiFiles = manifest.fileAncestry.filter((f: any) => f.upstream.startsWith("packages/tui/src/"))
    expect(appFiles.length).toBeGreaterThanOrEqual(4)
    expect(tuiFiles.length).toBeGreaterThanOrEqual(8)
  })

  for (const entry of JSON.parse(readFileSync(join(clientsRoot, "lineage.manifest.json"), "utf8")).fileAncestry) {
    test(`${entry.upstream} -> ${entry.destination}`, () => {
      const dest = join(clientsRoot, entry.destination)
      const text = readFileSync(dest, "utf8")
      expect(text.length).toBeGreaterThan(50)
      expect(text.toLowerCase()).toContain(entry.marker.toLowerCase().slice(0, 24))
      if (entry.upstreamBlob) {
        expect(entry.upstreamBlob).toMatch(/^[0-9a-f]{40}$/)
      }
    })
  }
})
