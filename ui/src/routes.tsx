import { lazy, ReactNode, Suspense } from 'react'
import { AlertTriangle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { createBrowserRouter, useParams, useRouteError } from 'react-router-dom'

import App, { StandaloneAIChatApp } from './App'
import { ProtectedRoute } from './components/protected-route'
import { Button } from './components/ui/button'
import { getSubPath } from './lib/subpath'
// Overview is the landing page, so it stays in the entry chunk: the first
// paint would need it anyway and an extra chunk round-trip would only slow
// the dashboard down.
import { Overview } from './pages/overview'

// Every other page is lazy-loaded so the entry bundle no longer carries the
// editors, terminals, helm views and detail pages a user may never open.
const LoginPage = lazy(() =>
  import('./pages/login').then((m) => ({ default: m.LoginPage }))
)
const SettingsPage = lazy(() =>
  import('./pages/settings').then((m) => ({ default: m.SettingsPage }))
)
const CRListPage = lazy(() =>
  import('./pages/cr-list-page').then((m) => ({ default: m.CRListPage }))
)
const HelmChartListPage = lazy(() =>
  import('./pages/helm-chart-list-page').then((m) => ({
    default: m.HelmChartListPage,
  }))
)
const HelmChartDetailPage = lazy(() =>
  import('./pages/helm-chart-detail-page').then((m) => ({
    default: m.HelmChartDetailPage,
  }))
)
const HelmReleaseListPage = lazy(() =>
  import('./pages/helmrelease-list-page').then((m) => ({
    default: m.HelmReleaseListPage,
  }))
)
const HelmReleaseDetail = lazy(() =>
  import('./pages/helmrelease-detail').then((m) => ({
    default: m.HelmReleaseDetail,
  }))
)
const ResourceDetail = lazy(() =>
  import('./pages/resource-detail').then((m) => ({ default: m.ResourceDetail }))
)
const ResourceList = lazy(() =>
  import('./pages/resource-list').then((m) => ({ default: m.ResourceList }))
)

const subPath = getSubPath()

// RouteLoadingFallback is shown while a lazy page chunk is being fetched; it
// intentionally matches the lightweight spinner used by the cluster gate.
function RouteLoadingFallback() {
  return (
    <div className="flex h-64 items-center justify-center">
      <div className="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600" />
    </div>
  )
}

// lazyPage wraps a lazy route element in the shared Suspense boundary.
function lazyPage(element: ReactNode) {
  return <Suspense fallback={<RouteLoadingFallback />}>{element}</Suspense>
}

// RouteErrorBoundary catches errors from any route below it — most notably a
// lazy chunk that fails to download (stale chunk hash after a redeploy, or a
// network hiccup). Before route-level code splitting this failure mode did
// not exist, so the router had no error UI and the user saw the router's
// bare default error screen; reloading is the reliable recovery.
function RouteErrorBoundary() {
  const { t } = useTranslation()
  const error = useRouteError()
  // Keep the raw error in the console for operators; the UI stays friendly.
  console.error('Route failed to load:', error)
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-4 px-4 text-center">
      <div className="flex h-12 w-12 items-center justify-center rounded-md bg-amber-50 text-amber-600 dark:bg-amber-950/40 dark:text-amber-300">
        <AlertTriangle className="h-6 w-6" aria-hidden="true" />
      </div>
      <div>
        <h1 className="text-lg font-semibold">{t('routeError.title')}</h1>
        <p className="mt-1 max-w-md text-sm text-muted-foreground">
          {t('routeError.description')}
        </p>
      </div>
      <Button variant="outline" onClick={() => window.location.reload()}>
        {t('routeError.reload')}
      </Button>
    </div>
  )
}

export const router = createBrowserRouter(
  [
    {
      // Pathless layout route: its errorElement catches failures from every
      // top-level route below, including lazy page chunk-load errors.
      errorElement: <RouteErrorBoundary />,
      children: [
        {
          path: '/login',
          element: lazyPage(<LoginPage />),
        },
        {
          path: '/ai-chat-box',
          element: (
            <ProtectedRoute>
              <StandaloneAIChatApp />
            </ProtectedRoute>
          ),
        },
        {
          path: '/',
          element: (
            <ProtectedRoute>
              <App />
            </ProtectedRoute>
          ),
          children: [
            {
              index: true,
              element: <Overview />,
            },
            {
              path: 'dashboard',
              element: <Overview />,
            },
            {
              path: 'settings',
              element: lazyPage(<SettingsPage />),
            },
            {
              path: 'crds/:crd',
              element: lazyPage(<CRListPage />),
            },
            {
              path: 'charts',
              element: lazyPage(<HelmChartListPage />),
            },
            {
              path: 'charts/:repository/:name',
              element: lazyPage(<HelmChartDetailPage />),
            },
            {
              path: 'helmreleases',
              element: lazyPage(<HelmReleaseListPage />),
            },
            {
              path: 'helmrelease/:namespace/:name',
              element: <HelmReleaseRoute />,
            },
            {
              path: 'crds/:resource/:namespace/:name',
              element: lazyPage(<ResourceDetail />),
            },
            {
              path: 'crds/:resource/:name',
              element: lazyPage(<ResourceDetail />),
            },
            {
              path: ':resource/:name',
              element: lazyPage(<ResourceDetail />),
            },
            {
              path: ':resource',
              element: lazyPage(<ResourceList />),
            },
            {
              path: ':resource/:namespace/:name',
              element: lazyPage(<ResourceDetail />),
            },
          ],
        },
      ],
    },
  ],
  {
    basename: subPath,
  }
)

function HelmReleaseRoute() {
  const { namespace = '', name = '' } = useParams()
  return lazyPage(<HelmReleaseDetail namespace={namespace} name={name} />)
}
