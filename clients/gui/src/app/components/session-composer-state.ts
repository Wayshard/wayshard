// Wayshard session composer state.
//
// Adapted from the imported OpenCode session composer state
// (third_party/opencode-v1.18.31/packages/app/src/pages/session/composer/session-composer-state.ts):
// the composer state surface is retained (draft options and request/run
// presence). OpenCode's provider/model/permission composer state is replaced by
// the Wayshard routing profile, artifact-only option and server-owned run state.
import { createSignal } from "solid-js"
import { useWayshard } from "../../wayshard/state"

export function createComposerState() {
  const ws = useWayshard()
  const [profile, setProfile] = createSignal("auto")
  const [artifactOnly, setArtifactOnly] = createSignal(false)

  return {
    prompt: {
      profile,
      setProfile,
      artifactOnly,
      setArtifactOnly,
    },
    request: {
      busy: () => ws.state.busy,
      run: () => ws.state.run,
      running: () => ws.state.run?.status === "running" || ws.state.run?.status === "planning",
      retryable: () => ws.state.run?.status === "failed" || ws.state.run?.status === "blocked",
    },
  }
}
