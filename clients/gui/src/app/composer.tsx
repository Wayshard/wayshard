// Wayshard graphical composer.
//
// Adapted from the imported OpenCode 2 composer foundation
// (third_party/opencode-v1.18.31/packages/session-ui/src/v2/components/prompt-input):
// the live editor, interaction machine, store, attachment presentation, history
// and suggestion plumbing are the imported PromptInputV2 component. OpenCode
// model/provider/permission concepts are replaced with Wayshard routing profile,
// artifact-only, run status, cancel and retry.
import { createStore } from "solid-js/store"
import { Button } from "@wayshard/ui/button"
import { Icon } from "@wayshard/ui/icon"
import { Show } from "solid-js"
import { PromptInputV2, type PromptInputV2PersistedState, type PromptInputV2Suggestion } from "@wayshard/gui/session-ui/v2/components/prompt-input"
import { createPromptInputV2Controller } from "@wayshard/gui/session-ui/v2/components/prompt-input/interaction"
import { useWayshard } from "../wayshard/state"
import { createComposerState } from "./components/session-composer-state"

export interface ComposerSubmit {
  text: string
  profile: string
  artifactOnly: boolean
}

export function Composer(props: { onSubmit: (input: ComposerSubmit) => void; onCancel: () => void }) {
  const ws = useWayshard()
  const [store, setStore] = createStore<PromptInputV2PersistedState>({
    prompt: [{ type: "text", content: "", start: 0, end: 0 }],
    context: { items: [] },
  })
  const composerState = createComposerState()

  const controller = createPromptInputV2Controller({
    store: () => [store, setStore],
    commands: () => [] as PromptInputV2Suggestion[],
    context: () => [] as PromptInputV2Suggestion[],
    searchContextFiles: () => [] as PromptInputV2Suggestion[],
    view: {
      placeholder: () => "Describe a task…",
      add: { onAttach: () => {} },
      submit: {
        stopping: () => false,
        working: () => ws.state.busy,
        onSubmit: () => submit(),
        onStop: () => props.onCancel(),
      },
    },
  })

  function text(): string {
    return store.prompt
      .map((part) => ("content" in part ? part.content : ""))
      .join("")
      .trim()
  }

  function reset() {
    setStore("prompt", [{ type: "text", content: "", start: 0, end: 0 }])
    setStore("cursor", 0)
  }

  function submit() {
    const value = text()
    if (!value) return
    props.onSubmit({ text: value, profile: composerState.prompt.profile(), artifactOnly: composerState.prompt.artifactOnly() })
    reset()
  }

  return (
    <div class="wh-composer" data-component="composer">
      <div class="wh-composer-editor">
        <PromptInputV2 controller={controller} disabled={ws.state.busy} />
      </div>
      <div class="wh-composer-actions">
        <label class="wh-checkbox">
          <input type="checkbox" checked={composerState.prompt.artifactOnly()} onChange={(e) => composerState.prompt.setArtifactOnly(e.currentTarget.checked)} />
          Artifact only
        </label>
        <label class="wh-field">
          <span class="wh-muted">Profile</span>
          <select class="wh-select" value={composerState.prompt.profile()} onChange={(e) => composerState.prompt.setProfile(e.currentTarget.value)}>
            <option value="auto">Auto</option>
            <option value="quality">Quality</option>
            <option value="speed">Speed</option>
            <option value="economy">Economy</option>
          </select>
        </label>
        <Show when={ws.state.run}>
          <span class="wh-muted">Run {ws.state.run!.status}</span>
        </Show>
        <div class="wh-composer-spacer" />
        <Show when={ws.state.run && (ws.state.run!.status === "running" || ws.state.run!.status === "planning")}>
          <Button size="small" variant="secondary" icon="close-small" onClick={() => props.onCancel()}>
            Cancel
          </Button>
        </Show>
        <Show when={ws.state.run && (ws.state.run!.status === "failed" || ws.state.run!.status === "blocked")}>
          <Button size="small" variant="secondary" icon="arrow-up" onClick={() => void ws.retryRun()}>
            Retry
          </Button>
        </Show>
        <Button variant="primary" size="small" icon="arrow-up" onClick={submit} disabled={ws.state.busy}>
          Send
        </Button>
      </div>
      <span class="wh-muted wh-composer-hint">
        <Icon name="prompt" size="small" /> Enter to send · Shift+Enter for a new line
      </span>
    </div>
  )
}
