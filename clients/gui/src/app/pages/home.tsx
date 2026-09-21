// Wayshard Home route.
//
// Adapted from the imported OpenCode application home route
// (third_party/opencode-v1.18.31/packages/app/src/pages/home.tsx): the
// application-level composition is retained — a raised rounded surface holding a
// scroll view with a responsive projects/sessions grid and a narrow-width
// utility nav — while the data comes from the Wayshard server.
import { ScrollView } from "@wayshard/ui/scroll-view"
import { useLocation } from "../router"
import { HomeProjects } from "./home/home-projects"
import { HomeSessions } from "./home/home-sessions"
import { HomeUtilityNav } from "./home/home-utility-nav"

export function Home() {
  const location = useLocation()
  return (
    <div
      data-component="home"
      data-route={location.pathname}
      class="m-2 min-h-0 flex-1 self-stretch overflow-hidden rounded-[10px] bg-v2-background-bg-base shadow-[var(--v2-elevation-raised)]"
    >
      <ScrollView class="h-full [container-type:size]">
        <div class="mx-auto grid min-h-full w-full max-w-[1080px] grid-rows-[auto_minmax(0,1fr)_auto] gap-4 px-3 lg:grid-cols-[280px_minmax(0,720px)] lg:grid-rows-1 lg:gap-8 lg:px-6">
          <HomeProjects />
          <HomeSessions />
          <HomeUtilityNav class="flex lg:hidden" />
        </div>
      </ScrollView>
    </div>
  )
}
