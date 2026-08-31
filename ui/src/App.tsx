import './App.css'

import { lazy, ReactNode, Suspense, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Outlet, useSearchParams } from 'react-router-dom'

import { AppSidebar } from './components/app-sidebar'
import { GlobalSearch } from './components/global-search'
import {
  GlobalSearchProvider,
  useGlobalSearch,
} from './components/global-search-provider'
import { SiteHeader } from './components/site-header'
import { SidebarInset, SidebarProvider } from './components/ui/sidebar'
import { Toaster } from './components/ui/sonner'
import { AIChatProvider } from './contexts/ai-chat-context'
import { ClusterProvider } from './contexts/cluster-context'
import { TerminalProvider, useTerminal } from './contexts/terminal-context'
import { useCluster } from './hooks/use-cluster'
import { apiClient } from './lib/api-client'

// The AI chatbox pulls in react-markdown and friends, and the floating
// terminal pulls in xterm.js plus its addons. Both are closed by default, so
// they are loaded on demand instead of weighing down the entry chunk.
const AIChatbox = lazy(() =>
  import('./components/ai-chat/ai-chatbox').then((m) => ({
    default: m.AIChatbox,
  }))
)
const StandaloneAIChatbox = lazy(() =>
  import('./components/ai-chat/ai-chatbox').then((m) => ({
    default: m.StandaloneAIChatbox,
  }))
)
const FloatingTerminal = lazy(() =>
  import('./components/floating-terminal').then((m) => ({
    default: m.FloatingTerminal,
  }))
)

function ClusterGate({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const { currentCluster, isLoading, error } = useCluster()

  useEffect(() => {
    apiClient.setClusterProvider(() => {
      return currentCluster || localStorage.getItem('current-cluster')
    })
  }, [currentCluster])

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="flex items-center space-x-2">
          <div className="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600" />
          <span>{t('cluster.loading')}</span>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="text-red-500">
          <p>{t('cluster.error', { error: error.message })}</p>
        </div>
      </div>
    )
  }

  return <>{children}</>
}

function AppContent() {
  const { isOpen, closeSearch } = useGlobalSearch()
  const { isOpen: isTerminalOpen } = useTerminal()
  const [searchParams] = useSearchParams()
  const isIframe = searchParams.get('iframe') === 'true'

  if (isIframe) {
    return <Outlet />
  }

  return (
    <>
      <SidebarProvider>
        <AppSidebar variant="inset" />
        <SidebarInset className="h-screen overflow-y-auto scrollbar-hide">
          <SiteHeader />
          <div>
            <div className="flex flex-col gap-4 py-4 md:gap-6">
              <div className="px-4 lg:px-6">
                <Outlet />
              </div>
            </div>
          </div>
        </SidebarInset>
      </SidebarProvider>
      <GlobalSearch open={isOpen} onOpenChange={closeSearch} />
      {isTerminalOpen ? (
        // The terminal chunk only downloads when the terminal is first
        // opened; while it loads nothing is rendered (the panel appears).
        <Suspense fallback={null}>
          <FloatingTerminal />
        </Suspense>
      ) : null}
      <Suspense fallback={null}>
        <AIChatbox />
      </Suspense>
      <Toaster />
    </>
  )
}

function AppProviders({ children }: { children: ReactNode }) {
  return (
    <TerminalProvider>
      <ClusterProvider>
        <GlobalSearchProvider>
          <AIChatProvider>{children}</AIChatProvider>
        </GlobalSearchProvider>
      </ClusterProvider>
    </TerminalProvider>
  )
}

function App() {
  return (
    <AppProviders>
      <ClusterGate>
        <AppContent />
      </ClusterGate>
    </AppProviders>
  )
}

export function StandaloneAIChatApp() {
  return (
    <AppProviders>
      <ClusterGate>
        {/* The standalone route exists for the chat UI, so its chunk is
            expected to load here; a spinner keeps the wait visible. */}
        <Suspense
          fallback={
            <div className="flex h-screen items-center justify-center">
              <div className="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600" />
            </div>
          }
        >
          <StandaloneAIChatbox />
        </Suspense>
      </ClusterGate>
    </AppProviders>
  )
}

export default App
