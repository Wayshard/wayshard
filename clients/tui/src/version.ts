// Wayshard TUI version identity.
//
// The release build injects the real release version and commit with Bun's
// `define` (see build.ts). When the TUI runs from source — `bun src/index.tsx` —
// those identifiers are absent, so we fall back to a development marker. The
// `typeof` guards are required because an undeclared identifier is only safe to
// reference through `typeof`.
declare const __WAYSHARD_TUI_VERSION__: string | undefined
declare const __WAYSHARD_TUI_COMMIT__: string | undefined

export const TUI_VERSION: string =
  typeof __WAYSHARD_TUI_VERSION__ === "string" ? __WAYSHARD_TUI_VERSION__ : "0.0.0-dev"

export const TUI_COMMIT: string =
  typeof __WAYSHARD_TUI_COMMIT__ === "string" ? __WAYSHARD_TUI_COMMIT__ : "unknown"

export function versionLine(): string {
  return `Wayshard TUI ${TUI_VERSION} (${TUI_COMMIT})`
}
