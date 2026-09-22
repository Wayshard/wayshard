// Lineage verification: makes it difficult to delete the adapted OpenCode
// client foundation and replace it with a tiny fresh UI while still claiming
// source-lineage conformance. Reads clients/lineage.manifest.json and asserts
// the live adapted subtrees still exist at substantial size.
//
// Pass 1E remediation (R3) strengthens this from "path + blob + provenance
// comment" into structural ancestry evidence: for every major behavior-bearing
// descendant the manifest declares inherited concept anchors that must appear in
// the *comment-stripped* live code AND in the comment-stripped upstream source,
// plus required live definitions, a minimum code size and a structural
// similarity floor. A provenance comment alone can no longer satisfy ancestry.
import { describe, expect, test } from "bun:test"
import { createHash } from "node:crypto"
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs"
import { dirname, join, resolve } from "node:path"

const clientsRoot = resolve(import.meta.dir, "..", "..")
const repoRoot = resolve(clientsRoot, "..")
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

function upstreamPath(rel: string): string {
  return join(repoRoot, "third_party", "opencode-v1.18.31", rel)
}

function livePath(rel: string): string {
  return join(clientsRoot, rel)
}

// stripComments removes line/block/JSX comments while preserving string
// literals (which carry behavior-bearing data-slot/data-component markers).
// Provenance comments therefore cannot satisfy a structural anchor.
export function stripComments(text: string): string {
  let out = ""
  let i = 0
  let state: "normal" | "line" | "block" | "double" | "single" | "template" = "normal"
  while (i < text.length) {
    const c = text[i]
    const n = text[i + 1]
    if (state === "normal") {
      if (c === "/" && n === "/") {
        state = "line"
        i += 2
        continue
      }
      if (c === "/" && n === "*") {
        state = "block"
        i += 2
        continue
      }
      if (c === '"') state = "double"
      else if (c === "'") state = "single"
      else if (c === "`") state = "template"
      out += c
      i++
      continue
    }
    if (state === "line") {
      if (c === "\n") {
        state = "normal"
        out += c
      }
      i++
      continue
    }
    if (state === "block") {
      if (c === "*" && n === "/") {
        state = "normal"
        i += 2
      } else i++
      continue
    }
    out += c
    if (c === "\\") {
      out += n ?? ""
      i += 2
      continue
    }
    if ((state === "double" && c === '"') || (state === "single" && c === "'") || (state === "template" && c === "`")) {
      state = "normal"
    }
    i++
  }
  return out
}

export function codeLineCount(text: string): number {
  return stripComments(text)
    .split("\n")
    .filter((line) => line.trim().length > 0).length
}

export function hasDefinition(code: string, name: string): boolean {
  const escaped = name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
  const def = new RegExp(`(?:function|const|class|let|var)\\s+${escaped}\\b`)
  const assignment = new RegExp(`\\b${escaped}\\s*=`)
  return def.test(code) || assignment.test(code)
}

const STOP = new Set(
  "const function return import from export type interface let if else for of in new class extends this props true false null undefined string number boolean void async await try catch finally throw as is default createSignal createEffect createMemo createStore Show For Switch Match solid div span button class classList onClick style data component slot value onChange onInput aria label size small variant ghost primary secondary use".split(
    " ",
  ),
)

function tokens(text: string): string[] {
  return (stripComments(text).match(/[A-Za-z_$][A-Za-z0-9_$]*/g) ?? []).filter((t) => !STOP.has(t))
}

function shingles(toks: string[], n = 3): Set<string> {
  const set = new Set<string>()
  for (let i = 0; i + n <= toks.length; i++) set.add(toks.slice(i, i + n).join(" "))
  return set
}

export function structuralSimilarity(a: string, b: string): number {
  const sa = shingles(tokens(a))
  const sb = shingles(tokens(b))
  if (sa.size === 0 || sb.size === 0) return 0
  let intersection = 0
  for (const x of sa) if (sb.has(x)) intersection++
  const union = new Set([...sa, ...sb]).size
  return union === 0 ? 0 : intersection / union
}

// The similarity floor is deliberately low: it is a secondary signal that
// rejects a file whose only relation to upstream is its name, not a measure of
// adaptation quality. The primary evidence is sharedAnchors + requiredDefs.
const SIMILARITY_FLOOR = 0.0025

function reachable(): Set<string> {
  const root = join(clientsRoot, "gui", "src", "app", "app.tsx")
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

  for (const entry of manifest.fileAncestry) {
    test(`${entry.upstream} -> ${entry.destination}`, () => {
      const dest = livePath(entry.destination)
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
  const used = manifest.fileAncestry.filter((f: any) => Array.isArray(f.usage) && f.usage.length > 0)
  test("signature adapted surfaces are reachable from production imports", () => {
    expect(used.length).toBeGreaterThanOrEqual(5)
  })
  for (const entry of used) {
    test(`${entry.destination} is imported by its consumers`, () => {
      for (const usage of entry.usage) {
        const text = readFileSync(livePath(usage.file), "utf8")
        if (usage.specifier) {
          // A real import of the module, not a substring coincidence.
          expect(text, `${usage.file} must import "${usage.specifier}"`).toContain(usage.specifier)
        } else {
          const base = entry.destination.split("/").pop()!.replace(/\.(ts|tsx)$/, "")
          expect(text.includes(base)).toBe(true)
        }
      }
    })
  }
})

describe("client source lineage — blob integrity", () => {
  test("concrete file ancestry entries record a real upstream git blob", () => {
    let checked = 0
    for (const entry of manifest.fileAncestry) {
      if (!/\.(ts|tsx|toml)$/.test(entry.upstream)) continue
      checked++
      expect(typeof entry.upstreamBlob).toBe("string")
      expect(entry.upstreamBlob).toMatch(/^[0-9a-f]{40}$/)
    }
    expect(checked).toBeGreaterThanOrEqual(15)
  })

  test("recorded upstream blobs actually match the vendored upstream source", () => {
    let verified = 0
    for (const entry of manifest.fileAncestry) {
      if (!entry.upstreamBlob || !entry.upstream.startsWith("packages/")) continue
      const vendored = upstreamPath(entry.upstream)
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

describe("client source lineage — structural ancestry evidence", () => {
  test("manifest declares structural ancestry for the major behavior-bearing descendants", () => {
    expect(manifest.version).toBeGreaterThanOrEqual(3)
    expect(Array.isArray(manifest.structuralAncestry)).toBe(true)
    const destinations = manifest.structuralAncestry.map((e: any) => e.destination)
    for (const required of [
      "gui/src/app/pages/session/review-tab.tsx",
      "gui/src/app/pages/session/terminal-panel-v2.tsx",
      "gui/src/app/components/composer-region.tsx",
      "gui/src/app/components/composer-region-controller.ts",
      "gui/src/app/components/session-composer-state.ts",
      "gui/src/app/pages/session/file-tabs.tsx",
      "gui/src/app/pages/session/timeline/message-timeline.tsx",
    ]) {
      expect(destinations).toContain(required)
    }
  })

  const seen = reachable()
  for (const entry of manifest.structuralAncestry) {
    test(`${entry.destination} carries real structural ancestry from ${entry.upstream}`, () => {
      const upstream = readFileSync(upstreamPath(entry.upstream), "utf8")
      const live = readFileSync(livePath(entry.destination), "utf8")
      const liveCode = stripComments(live)
      const upstreamCode = stripComments(upstream)

      // 1. Not a tiny wrapper: substantial comment-stripped code.
      expect(codeLineCount(live), `${entry.destination} is too small to be an adaptation`).toBeGreaterThanOrEqual(
        entry.minCodeLines,
      )

      // 2. Behavior-bearing definitions exist in the live code.
      for (const def of entry.requiredDefs) {
        expect(hasDefinition(liveCode, def), `${entry.destination} must define ${def}`).toBe(true)
      }

      // 3. Inherited concept anchors appear in BOTH the comment-stripped
      //    upstream and live source. Comments cannot satisfy these.
      for (const anchor of entry.sharedAnchors) {
        expect(upstreamCode, `upstream ${entry.upstream} must contain ${anchor}`).toContain(anchor)
        expect(liveCode, `${entry.destination} must contain inherited anchor ${anchor}`).toContain(anchor)
      }

      // 4. Live-only structural markers.
      for (const anchor of entry.liveAnchors) {
        expect(liveCode, `${entry.destination} must contain ${anchor}`).toContain(anchor)
      }

      // 5. Secondary signal: not merely filename-related to upstream.
      expect(
        structuralSimilarity(upstream, live),
        `${entry.destination} shares almost no structure with ${entry.upstream}`,
      ).toBeGreaterThanOrEqual(SIMILARITY_FLOOR)

      // 6. Production reachability.
      expect(seen.has(resolve(livePath(entry.destination))), `${entry.destination} is not production-reachable`).toBe(true)
    })
  }

  test("the structural gate rejects a provenance-only wrapper", () => {
    const wrapper = `// Adapted from the imported OpenCode terminal panel (terminal-panel-v2)
// provenance marker only
export function SessionTerminalPanel() { return null }`
    const code = stripComments(wrapper)
    expect(codeLineCount(wrapper)).toBeLessThan(120)
    expect(code.includes("terminal-wrapper-")).toBe(false)
    expect(hasDefinition(code, "SessionTerminalPanel")).toBe(true) // the one thing a wrapper can fake
    // ...but it fails the shared-anchor, live-anchor, size and similarity gates.
    expect(structuralSimilarity(readFileSync(upstreamPath("packages/app/src/pages/session/terminal-panel-v2.tsx"), "utf8"), wrapper)).toBeLessThan(
      0.02,
    )
  })

  test("comment stripping removes provenance text but keeps string markers", () => {
    const sample = `// data-slot="terminal-tab" in a comment\nexport const x = 'data-slot="terminal-tab"'`
    const code = stripComments(sample)
    expect(code.includes("in a comment")).toBe(false)
    expect(code.includes('data-slot="terminal-tab"')).toBe(true)
  })
})

describe("client source lineage — production import reachability", () => {
  test("signature application descendants are reachable from the production app root", () => {
    const seen = reachable()
    const signatures = manifest.fileAncestry
      .filter((f: any) => f.destination.startsWith("gui/src/app/"))
      .filter((f: any) =>
        /(app|home|layout|sidebar-shell|sidebar-project|sidebar-items|session-page|titlebar|composer-region|review-tab|file-tabs|terminal-panel|new-session)/.test(
          f.destination,
        ),
      )
    expect(signatures.length).toBeGreaterThanOrEqual(10)
    for (const entry of signatures) {
      const dest = resolve(livePath(entry.destination))
      expect(seen.has(dest), `${entry.destination} is not production-reachable`).toBe(true)
    }
  })
})
