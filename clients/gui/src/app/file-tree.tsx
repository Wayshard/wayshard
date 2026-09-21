// Wayshard file tree.
//
// Adapted from the imported OpenCode 2 graphical client file tree
// (third_party/opencode-v1.18.31/packages/app/src/components/file-tree-v2.tsx
// and file-tree-v2-model.ts): the tree model, expand/collapse flattening,
// directory-first ordering and file icons are retained. Change badges and the
// Wayshard selection/editor integration are Wayshard.
import { For, createMemo, createSignal } from "solid-js"
import { FileIcon } from "@wayshard/ui/file-icon"
import { Icon } from "@wayshard/ui/icon"
import { Tag } from "@wayshard/ui/tag"
import { buildFileTreeV2Model, flattenFileTreeV2 } from "./file-tree-model"

export function FileTree(props: {
  paths: string[]
  selected?: string
  changed?: Record<string, string>
  onSelect: (path: string) => void
}) {
  const model = createMemo(() => buildFileTreeV2Model(props.paths))
  const [expanded, setExpanded] = createSignal<Set<string>>(new Set())
  const rows = createMemo(() => flattenFileTreeV2(model(), (path) => expanded().has(path)))

  function toggle(path: string) {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(path)) next.delete(path)
      else next.add(path)
      return next
    })
  }

  return (
    <ul class="wh-tree">
      <For each={rows()}>
        {(row) => (
          <li>
            <button
              class="wh-tree-row"
              style={{ "padding-left": `${8 + row.level * 14}px` }}
              data-active={row.node.type === "file" && row.node.path === props.selected}
              data-dir={row.node.type === "directory"}
              onClick={() => (row.node.type === "directory" ? toggle(row.node.path) : props.onSelect(row.node.path))}
            >
              {row.node.type === "directory" ? (
                <Icon name="chevron-right" size="small" />
              ) : (
                <FileIcon node={{ path: row.node.path, type: "file" }} class="wh-tree-icon" />
              )}
              <span class="wh-truncate">{row.node.name}</span>
              {row.node.type === "file" && props.changed?.[row.node.path] ? (
                <Tag>{props.changed[row.node.path]}</Tag>
              ) : null}
            </button>
          </li>
        )}
      </For>
    </ul>
  )
}
