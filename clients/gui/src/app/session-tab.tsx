// Wayshard session/file tab.
//
// Adapted from the imported OpenCode 2 graphical client session tab
// (third_party/opencode-v1.18.31/packages/app/src/components/session/session-sortable-tab.tsx):
// the FileVisual + tab presentation is retained. Drag-and-drop and the OpenCode
// file/command contexts are replaced with Wayshard equivalents.
import { Show, createMemo, type JSX } from "solid-js"
import { FileIcon } from "@wayshard/ui/file-icon"
import { IconButton } from "@wayshard/ui/icon-button"
import { getFilename } from "@wayshard/gui/session-ui/util/path"
import { useCommand } from "./command"

export function FileVisual(props: { path: string; active?: boolean; temporary?: boolean }): JSX.Element {
  return (
    <div class="wh-tab-visual">
      <Show
        when={!props.active}
        fallback={<FileIcon node={{ path: props.path, type: "file" }} class="wh-tab-icon" />}
      >
        <span class="wh-tab-icon-wrap">
          <FileIcon node={{ path: props.path, type: "file" }} class="wh-tab-icon" />
        </span>
      </Show>
      <span class="wh-tab-label" classList={{ "wh-tab-temporary": props.temporary }}>
        {getFilename(props.path)}
      </span>
    </div>
  )
}

export function SessionTab(props: {
  tab: string
  path?: string
  temporary?: boolean
  active?: boolean
  onClose: (tab: string) => void
}): JSX.Element {
  const command = useCommand()
  const content = createMemo(() => (props.path ? <FileVisual path={props.path} temporary={props.temporary} active={props.active} /> : undefined))
  return (
    <div class="wh-tab" data-active={props.active}>
      <Show when={content()}>{(value) => value()}</Show>
      <IconButton
        icon="close-small"
        variant="ghost"
        class="wh-tab-close"
        title={`Close tab (${command.keybind("tab.close") ?? "mod+w"})`}
        onClick={() => props.onClose(props.tab)}
        aria-label="Close tab"
      />
    </div>
  )
}
