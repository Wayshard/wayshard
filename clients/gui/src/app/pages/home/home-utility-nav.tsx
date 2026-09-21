// Wayshard Home utility navigation (narrow widths).
//
// Adapted from the imported OpenCode application home utility nav
// (third_party/opencode-v1.18.31/packages/app/src/pages/home/home-projects-view.tsx
// HomeUtilityNav): the narrow-width settings/help affordances are retained and
// open the Wayshard settings surface using the inherited dialog pattern.
import { Button } from "@wayshard/ui/button"
import { Dialog } from "@wayshard/ui/dialog"
import { useDialog } from "@wayshard/ui/context/dialog"
import { SettingsView } from "../../views"

export function HomeUtilityNav(props: { class?: string }) {
  const dialog = useDialog()
  function settings() {
    dialog.show(() => (
      <Dialog title="Settings" size="large">
        <div class="wh-dialog-surface">
          <SettingsView />
        </div>
      </Dialog>
    ))
  }
  return (
    <nav class={`${props.class ?? ""} items-center gap-1`}>
      <Button size="small" variant="ghost" icon="settings-gear" onClick={settings}>
        Settings
      </Button>
      <Button size="small" variant="ghost" icon="prompt" onClick={() => window.open("https://wayshard.dev", "_blank", "noopener")}>
        Help
      </Button>
    </nav>
  )
}
