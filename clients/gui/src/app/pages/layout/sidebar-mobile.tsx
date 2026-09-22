// Wayshard narrow-width sidebar navigation.
//
// Adapted from the imported OpenCode application narrow-width navigation
// (third_party/opencode-v1.18.31/packages/app/src/pages/layout.tsx): the
// composition is retained — below the `xl` breakpoint the persistent sidebar is
// hidden and a `sidebar-nav-mobile` overlay slides in from the start edge, with
// a scrim that closes it on click. State comes from the layout context
// `mobileSidebar` (opened/hide), matching upstream.
import { type JSX } from "solid-js"
import { useLayout } from "../../context/layout"

export function SidebarMobile(props: { children: JSX.Element }): JSX.Element {
  const layout = useLayout()
  return (
    <div class="xl:hidden">
      <div
        data-component="sidebar-mobile-scrim"
        classList={{
          "fixed inset-x-0 top-10 bottom-0 z-40 transition-opacity duration-200": true,
          "opacity-100 pointer-events-auto": layout.mobileSidebar.opened(),
          "opacity-0 pointer-events-none": !layout.mobileSidebar.opened(),
        }}
        onClick={(e) => {
          if (e.target === e.currentTarget) layout.mobileSidebar.hide()
        }}
      />
      <nav
        aria-label="Projects and sessions"
        data-component="sidebar-nav-mobile"
        classList={{
          "fixed top-10 bottom-0 start-0 z-50 w-full max-w-[400px] overflow-hidden border-e border-v2-border-border-base bg-v2-background-bg-base transition-transform duration-200 ease-out": true,
          "translate-x-0": layout.mobileSidebar.opened(),
          "-translate-x-full": !layout.mobileSidebar.opened(),
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {props.children}
      </nav>
    </div>
  )
}
