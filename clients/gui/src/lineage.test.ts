// Lineage verification: makes it difficult to delete the adapted OpenCode
// client foundation and replace it with a tiny fresh UI while still claiming
// source-lineage conformance. Reads clients/lineage.manifest.json and asserts
// the live adapted subtrees still exist at substantial size.
import { describe, expect, test } from "bun:test"
import { createHash } from "node:crypto"
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs"
import { dirname, join, resolve } from "node:path"

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

describe("client source lineage — production usage", () => {
  const manifest2 = JSON.parse(readFileSync(join(clientsRoot, "lineage.manifest.json"), "utf8"))
  const used = manifest2.fileAncestry.filter((f: any) => Array.isArray(f.usage) && f.usage.length > 0)
  test("signature adapted surfaces are reachable from production imports", () => {
    expect(used.length).toBeGreaterThanOrEqual(5)
  })
  for (const entry of used) {
    test(`${entry.destination} is used by ${entry.usage.join(", ")}`, () => {
      const base = entry.destination.split("/").pop()!.replace(/\.(ts|tsx)$/, "")
      for (const usage of entry.usage) {
        const text = readFileSync(join(clientsRoot, usage), "utf8")
        const references = text.includes(base) || text.includes(entry.marker.slice(0, 20))
        expect(references).toBe(true)
      }
    })
  }
})

describe("client source lineage — blob integrity", () => {
  const manifest3 = JSON.parse(readFileSync(join(clientsRoot, "lineage.manifest.json"), "utf8"))
  test("concrete file ancestry entries record a real upstream git blob", () => {
    let checked = 0
    for (const entry of manifest3.fileAncestry) {
      if (!/\.(ts|tsx|toml)$/.test(entry.upstream)) continue
      checked++
      expect(typeof entry.upstreamBlob).toBe("string")
      expect(entry.upstreamBlob).toMatch(/^[0-9a-f]{40}$/)
    }
    expect(checked).toBeGreaterThanOrEqual(15)
  })

  test("recorded upstream blobs actually match the vendored upstream source", () => {
    const repoRoot = resolve(clientsRoot, "..")
    let verified = 0
    for (const entry of manifest3.fileAncestry) {
      if (!entry.upstreamBlob || !entry.upstream.startsWith("packages/")) continue
      const vendored = join(repoRoot, "third_party", "opencode-v1.18.31", entry.upstream)
      if (!existsSync(vendored)) continue
      const data = readFileSync(vendored)
      const header = Buffer.from(`blob ${data.length}\0`)
      const hash = createHash("sha1").update(Buffer.concat([header, data])).digest("hex")
      expect(hash, `${entry.upstream} blob mismatch`).toBe(entry.upstreamBlob)
      verified++
    }
    expect(verified).toBeGreaterThanOrEqual(15)
  })
})

describe("client source lineage — production import reachability", () => {
  const manifest4 = JSON.parse(readFileSync(join(clientsRoot, "lineage.manifest.json"), "utf8"))
  const root = resolve(clientsRoot, "gui", "src", "app", "app.tsx")

  function reachable(): Set<string> {
    const seen = new Set<string>()
    const queue = [root]
    while (queue.length) {
      const file = queue.pop()!
      if (seen.has(file)) continue
      seen.add(file)
      let text = ""
      try {
        text = readFileSync(file, "utf8")
      } catch {
        continue
      }
      const base = dirname(file)
      for (const m of text.matchAll(/from\s+"([^"]+)"/g)) {
        const spec = m[1]
        if (!spec.startsWith(".")) continue
        const target = resolve(base, spec)
        for (const cand of [target, `${target}.tsx`, `${target}.ts`, join(target, "index.tsx"), join(target, "index.ts")]) {
          if (candidateExists(cand)) {
            queue.push(cand)
            break
          }
        }
      }
    }
    return seen
  }

  function candidateExists(p: string): boolean {
    try {
      return statSync(p).isFile()
    } catch {
      return false
    }
  }

  test("signature application descendants are reachable from the production app root", () => {
    const seen = reachable()
    const signatures = manifest4.fileAncestry
      .filter((f: any) => f.destination.startsWith("gui/src/app/"))
      .filter((f: any) =>
        /(app|home|layout|sidebar-shell|sidebar-project|sidebar-items|session-page|titlebar|composer-region|review-tab|file-tabs|terminal-panel|new-session)/.test(
          f.destination,
        ),
      )
    expect(signatures.length).toBeGreaterThanOrEqual(10)
    for (const entry of signatures) {
      const dest = resolve(clientsRoot, entry.destination)
      expect(seen.has(dest), `${entry.destination} is not production-reachable`).toBe(true)
    }
  })
})
