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

### Theme pipeline

The live graphical app imports the adapted v2 theme layer
(`@wayshard/ui/v2/styles/tailwind.css`) and the adapted session-ui component
styles (`clients/gui/src/session-ui/styles/index.css`), and mounts the adapted
runtime `ThemeProvider` (`@wayshard/ui/theme/context`, default theme `oc-2`,
dark). This mirrors upstream `packages/app/src/index.css` and
`packages/app/src/app.tsx`; without it the v2 design tokens are undefined and
the client renders unreadably.

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

## Application-shell ancestry (per-file)

Beyond the component library, the Wayshard graphical **application shell** itself is
adapted from concrete OpenCode app files:

| OpenCode app file | Wayshard live file | Mode |
| --- | --- | --- |
| `packages/app/src/context/command.tsx` | `clients/gui/src/app/command.tsx` | adapted |
| `packages/app/src/pages/layout-new.tsx` | `clients/gui/src/app/layout.tsx` | adapted |
| `packages/app/src/components/session/session-sortable-tab.tsx` | `clients/gui/src/app/session-tab.tsx` | adapted |
| `packages/app/src/components/dialog-command-palette-v2.tsx` | `clients/gui/src/app/command-palette.tsx` | adapted |

The command keybinding parser/matcher/formatter, palette option resolution and
registration lifecycle; the titlebar + main + toast layout; the file/session tab
presentation; and the palette search/grouped-list/keybind behaviour are retained.
The command set, navigation hierarchy and domain are Wayshard.

## TUI component ancestry (per-file)

The Wayshard TUI adapts concrete OpenCode TUI component files:

| OpenCode TUI file | Wayshard live file | Mode |
| --- | --- | --- |
| `packages/tui/src/ui/dialog.tsx` | `clients/tui/src/ui/dialog.tsx` | copied+adapted |
| `packages/tui/src/ui/dialog-confirm.tsx` | `clients/tui/src/ui/dialog-confirm.tsx` | copied+adapted |
| `packages/tui/src/ui/dialog-prompt.tsx` | `clients/tui/src/ui/dialog-prompt.tsx` | copied+adapted |
| `packages/tui/src/ui/dialog-alert.tsx` | `clients/tui/src/ui/dialog-alert.tsx` | copied+adapted |
| `packages/tui/src/ui/toast.tsx` | `clients/tui/src/ui/toast.tsx` | copied+adapted |
| `packages/tui/src/ui/border.ts` | `clients/tui/src/ui/border.ts` | copied+adapted |
| `packages/tui/src/ui/spinner.ts` | `clients/tui/src/ui/spinner.ts` | copied+adapted |
| `packages/tui/src/component/spinner.tsx` | `clients/tui/src/component/spinner.tsx` | copied+adapted |
| `packages/tui/src/component/logo.tsx` | `clients/tui/src/component/logo.tsx` | copied+adapted |
| `packages/tui/src/theme/index.ts` | `clients/tui/src/theme/index.ts` | copied+adapted |
| `packages/tui/src/keymap.tsx` | `clients/tui/src/keymap.tsx` | adapted |
| `packages/tui/src/context/theme.tsx` | `clients/tui/src/context/theme.tsx` | adapted |
| `packages/tui/bunfig.toml` | `clients/tui/bunfig.toml` | copied |

## Manifest mechanics

`clients/lineage.manifest.json` (version 2) records:

- `subtrees`: broad upstream→destination mappings with minimum file/LOC sizes and
  required marker files.
- `fileAncestry`: concrete per-file mappings with the upstream path, the upstream
  git blob hash at the imported commit, the live destination, and a provenance
  marker that the live file must contain.

`clients/gui/src/lineage.test.ts` enforces both: adapted subtrees must remain at
substantial size, every `fileAncestry` destination must exist, be non-trivial and
contain its provenance marker, and no live client source may contain an
`@opencode-ai/` import specifier. This makes it impossible to delete the adapted
application/TUI foundation and replace it with a small fresh app while passing.

## Signature surface ancestry (final Pass 1E)

| OpenCode source | Wayshard live file | Mode | Production use |
| --- | --- | --- | --- |
| `packages/session-ui/src/v2/components/prompt-input/index.tsx` | `clients/gui/src/session-ui/v2/components/prompt-input/index.tsx` | copied+adapted | `clients/gui/src/app/composer.tsx` |
| `packages/session-ui/src/components/file.tsx` | `clients/gui/src/session-ui/components/file.tsx` | copied+adapted | `clients/gui/src/app/views.tsx` (Changes diff) |
| `packages/app/src/components/file-tree-v2-model.ts` | `clients/gui/src/app/file-tree-model.ts` | adapted | `clients/gui/src/app/file-tree.tsx` |
| `packages/app/src/components/file-tree-v2.tsx` | `clients/gui/src/app/file-tree.tsx` | adapted | Files view |
| `packages/app/src/components/terminal.tsx` | `clients/gui/src/app/terminal.tsx` | adapted | Terminal view (ghostty-web renderer) |
| `packages/tui/src/ui/dialog-select.tsx` | `clients/tui/src/ui/dialog-select.tsx` | copied+adapted | `clients/tui/src/app.tsx` (command palette) |

The composer renders the imported `PromptInputV2` editor/interaction machine;
Changes renders the imported `File` diff component; Files uses the adapted
file-tree model; the terminal renders through the imported ghostty-web
presentation against the Wayshard server-owned PTY; the TUI command palette uses
the adapted `DialogSelect`.

The lineage test additionally verifies production usage: each entry with a
`usage` list must be referenced by the listed live file, so the adapted
components cannot be present-but-unused.

## Application-level descendants (Pass 1E port)

The production graphical application composition now descends from the actual
OpenCode application source, with Wayshard adapted into it:

| OpenCode source | Wayshard live file |
| --- | --- |
| `packages/app/src/app.tsx` | `clients/gui/src/app/app.tsx` |
| `packages/app/src/pages/home.tsx` | `clients/gui/src/app/pages/home.tsx` |
| `packages/app/src/pages/home/home-projects-view.tsx` | `clients/gui/src/app/pages/home/home-projects.tsx` |
| `packages/app/src/pages/home/home-sessions-view.tsx` | `clients/gui/src/app/pages/home/home-sessions.tsx` |
| `packages/app/src/pages/layout.tsx` | `clients/gui/src/app/pages/layout.tsx` |
| `packages/app/src/pages/layout/sidebar-shell.tsx` | `clients/gui/src/app/pages/layout/sidebar-shell.tsx` |
| `packages/app/src/pages/layout/sidebar-project.tsx` | `clients/gui/src/app/pages/layout/sidebar-project.tsx` |
| `packages/app/src/pages/layout/sidebar-items.tsx` | `clients/gui/src/app/pages/layout/sidebar-items.tsx` |
| `packages/app/src/pages/session.tsx` | `clients/gui/src/app/session-page.tsx` |
| `packages/app/src/components/titlebar.tsx` | `clients/gui/src/app/components/titlebar.tsx` |
| `packages/app/src/pages/session/composer/session-composer-region.tsx` | `clients/gui/src/app/components/composer-region.tsx` |
| `packages/app/src/pages/session/review-tab.tsx` | `clients/gui/src/app/pages/session/review-tab.tsx` |
| `packages/app/src/pages/session/file-tabs.tsx` | `clients/gui/src/app/pages/session/file-tabs.tsx` |
| `packages/app/src/pages/session/terminal-panel-v2.tsx` | `clients/gui/src/app/pages/session/terminal-panel-v2.tsx` |
| `packages/app/src/pages/new-session/new-session-view.tsx` | `clients/gui/src/app/pages/new-session.tsx` |
| `packages/app/src/context/layout.tsx` | `clients/gui/src/app/context/layout.tsx` |
| `packages/app/src/pages/session/timeline/model.ts` | `clients/gui/src/app/pages/session/timeline/model.ts` |
| `packages/app/src/pages/session/timeline/message-timeline.tsx` | `clients/gui/src/app/pages/session/timeline/message-timeline.tsx` |
| `packages/app/src/pages/session/composer/session-composer-state.ts` | `clients/gui/src/app/components/session-composer-state.ts` |
| `packages/app/src/pages/layout.tsx` (narrow) | `clients/gui/src/app/pages/layout/sidebar-mobile.tsx` |

`clients/lineage.manifest.json` records each with its upstream git blob;
`clients/gui/src/lineage.test.ts` verifies the blob against the vendored source
and asserts production-import reachability from `app.tsx`. Routing uses the
adapted Wayshard router (`clients/gui/src/app/router.tsx`) because the inherited
`@solidjs/router` did not advance its reactive location inside the adapted
provider tree.
