// Tests for the adapted OpenCode file-tree model.
import { describe, expect, test } from "bun:test"
import { buildFileTreeV2Model, flattenFileTreeV2, normalizeFileTreeV2Path } from "./file-tree-model"

describe("wayshard file tree (adapted from packages/app/src/components/file-tree-v2-model.ts)", () => {
  test("normalizes paths", () => {
    expect(normalizeFileTreeV2Path("/a/b/")).toBe("a/b")
    expect(normalizeFileTreeV2Path("a\\b")).toBe("a/b")
    expect(normalizeFileTreeV2Path("a//b")).toBe("a/b")
  })

  test("builds a directory-first tree", () => {
    const model = buildFileTreeV2Model(["src/index.ts", "src/app.tsx", "README.md", "src/lib/util.ts"])
    const root = model.children.get("")!
    expect(root.map((n) => n.name)).toEqual(["src", "README.md"])
    expect(root[0].type).toBe("directory")
    const src = model.children.get("src")!
    expect(src.map((n) => n.name)).toEqual(["lib", "app.tsx", "index.ts"])
  })

  test("flatten respects expansion", () => {
    const model = buildFileTreeV2Model(["src/index.ts", "README.md"])
    const collapsed = flattenFileTreeV2(model, () => false)
    expect(collapsed.map((r) => r.node.name)).toEqual(["src", "README.md"])
    const expanded = flattenFileTreeV2(model, (p) => p === "src")
    expect(expanded.map((r) => r.node.name)).toEqual(["src", "index.ts", "README.md"])
    expect(expanded.find((r) => r.node.name === "index.ts")!.level).toBe(1)
  })
})
