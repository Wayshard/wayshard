# Client source lineage

Wayshard's graphical clients and TUI are **adapted from the imported OpenCode 2
client source**, not merely inspired by it. This document records the exact
lineage, what was retained, and what was deliberately replaced.

## Upstream

| Field | Value |
| --- | --- |
| Project | OpenCode 2 |
| Tag | `v1.18.31` |
| Commit | `014614d35b397775e5d397a490fc72368c894ec2` |
| Vendored path | `third_party/opencode-v1.18.31` |
| License | MIT (see `NOTICE`, `THIRD_PARTY_NOTICES.md`) |
| Role | Provenance / reference input only. Never a runtime or build input of the live clients. |

The vendored tree is immutable provenance. The live product uses **adapted
copies** under Wayshard-owned client directories. There is no upstream remote,
submodule, sync workflow, OpenCode API compatibility layer, or OpenCode runtime
dependency.

## Graphical client (Web / Desktop / Android)

One shared SolidJS application, `@wayshard/gui`, is mounted by Web and hosted by
the Tauri 2 Desktop and Android shells.

| OpenCode source | Wayshard destination | Mode |
| --- | --- | --- |
| `packages/ui/src` (design system: theme, tokens, styles, primitives, v2 components) | `clients/ui/src` (`@wayshard/ui`) | copied + adapted |
| `packages/session-ui/src` (message/part rendering, markdown, diffs, review) | `clients/gui/src/session-ui` | copied + adapted |
| `packages/app/src` (shell, layout, prompt, file tree, dialogs, terminal) | `clients/gui/src/app` + `clients/gui/src/wayshard` | adapted (structure/patterns) |

Adaptations:

- Package identity rebranded `@opencode-ai/ui` → `@wayshard/ui`.
- OpenCode SDK/core/client runtime imports replaced by a Wayshard view-model
  shim (`clients/gui/src/session-ui/sdk/model.ts`,
  `util/{binary,encode,path}.ts`) populated from `@wayshard/sdk`.
- OpenCode server/session/provider contexts, provider/model management UI,
  subscription copy and desktop Electron shell removed.
- Branding removed from live UI; attribution retained in legal notices.

## TUI

| OpenCode source | Wayshard destination | Mode |
| --- | --- | --- |
| `packages/tui/src/theme` (+ assets) | `clients/tui/src/theme` | copied + adapted |
| `packages/tui/src` (OpenTUI layout, key handling, dialogs, prompt, status) | `clients/tui/src/app.tsx` | adapted |
| `packages/tui/bunfig.toml` (OpenTUI Solid preload) | `clients/tui/bunfig.toml` | copied |

The TUI is a real OpenTUI/Solid terminal application (`@opentui/core`,
`@opentui/solid`, `@opentui/keymap`) — **not** a readline prompt. Domain and data
path are Wayshard (`@wayshard/sdk`); the theme system and assets are the imported
OpenCode theme system.

## Verification

`clients/lineage.manifest.json` records the subtree mapping and minimum live
sizes. `clients/gui/src/lineage.test.ts` asserts the adapted subtrees still
exist at substantial size, that the recorded markers exist, and that no live
client source contains an `@opencode-ai/` import specifier. This makes it
difficult to delete the adapted foundation and replace it with a small fresh UI
while still claiming conformance.

## Deliberate removals

- Provider/model connection and management UI (Wayshard coding providers are
  harness-owned; Wayshard never scrapes provider credentials).
- OpenCode server/session/provider contexts, sync reducers and optimistic
  session state (replaced by Wayshard's durable run/stage model and event
  stream).
- OpenCode SDK/core/client runtime packages.
- The OpenCode Electron desktop shell (Wayshard Desktop/Android are Tauri 2).
- OpenCode product branding, subscription copy and namespaces.
