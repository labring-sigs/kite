import Logo from '@/assets/icon.svg'
import { useAuth } from '@/contexts/auth-context'
import {
  AlertTriangle,
  Database,
  Info,
  KeyRound,
  Loader2,
  MonitorSmartphone,
  RefreshCcw,
  Server,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Navigate, useSearchParams } from 'react-router-dom'

import { Button } from '@/components/ui/button'
import { Footer } from '@/components/footer'
import { LanguageToggle } from '@/components/language-toggle'
import { ModeToggle } from '@/components/mode-toggle'

type AuthFaultReason =
  | 'unauthenticated'
  | 'session_refresh_failed'
  | 'authentication_failed'
  | 'logout'
  | 'insufficient_permissions'
  | 'token_exchange_failed'
  | 'user_info_failed'
  | 'jwt_generation_failed'
  | 'callback_failed'
  | 'callback_error'
  | 'user_disabled'
  | 'unknown'

const authFaultReasons = new Set<AuthFaultReason>([
  'unauthenticated',
  'session_refresh_failed',
  'authentication_failed',
  'logout',
  'insufficient_permissions',
  'token_exchange_failed',
  'user_info_failed',
  'jwt_generation_failed',
  'callback_failed',
  'callback_error',
  'user_disabled',
  'unknown',
])

const normalizeReason = (
  reason: string | null,
  error: string | null
): AuthFaultReason => {
  const value = reason || error || 'unknown'
  if (authFaultReasons.has(value as AuthFaultReason)) {
    return value as AuthFaultReason
  }
  return 'unknown'
}

export function LoginPage() {
  const { t } = useTranslation()
  const { user, isLoading, sealosSdkAccessStatus } = useAuth()
  const [searchParams] = useSearchParams()
  const showSealosSdkStatus =
    sealosSdkAccessStatus === 'checking' ||
    sealosSdkAccessStatus === 'unavailable'

  const reason = normalizeReason(
    searchParams.get('reason'),
    searchParams.get('error')
  )

  // Sealos deployments only accept sessions launched from the Sealos
  // desktop. When the SDK session channel is absent, this page becomes a
  // dedicated notice instead of the operator fault page (and the backend
  // never routes to /setup either): direct browser access can never
  // establish a session here.
  const sealosDesktopRequired = sealosSdkAccessStatus === 'unavailable'

  // The desktop-required branch narrows sealosSdkAccessStatus via control
  // flow, which would shrink the template-literal translation keys used for
  // the SDK status box below and break the typed t() calls; this full-union
  // copy keeps those key expressions exactly as they were.
  const sdkStatusForKeys: typeof sealosSdkAccessStatus = sealosSdkAccessStatus

  const checks = [
    {
      icon: Database,
      label: t('login.operatorCheckLabels.database'),
      description: t('login.operatorChecks.database'),
    },
    {
      icon: KeyRound,
      label: t('login.operatorCheckLabels.authConfig'),
      description: t('login.operatorChecks.authConfig'),
    },
    {
      icon: Server,
      label: t('login.operatorCheckLabels.backendLogs'),
      description: t('login.operatorChecks.backendLogs'),
    },
  ]

  if (user && !isLoading) {
    return <Navigate to="/" replace />
  }

  // Keep the spinner while the SDK channel probe is in flight so the
  // desktop-required notice cannot flash for users who actually did open
  // Kite from the Sealos desktop.
  if (isLoading || sealosSdkAccessStatus === 'checking') {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-32 w-32 border-b-2 border-primary"></div>
      </div>
    )
  }

  // Non-desktop access to a Sealos-only deployment: render the entry notice
  // with the same standalone shell (logo header + footer) as the fault page.
  if (sealosDesktopRequired) {
    return (
      <div className="min-h-screen bg-muted/20">
        <div className="flex min-h-screen flex-col">
          <header className="flex h-[var(--header-height)] shrink-0 items-center border-b bg-background px-4 lg:px-6">
            <div className="flex items-center gap-2">
              <img src={Logo} className="h-8 w-8 dark:invert" alt="Kite" />
              <div className="min-w-0">
                <div className="text-base font-semibold leading-none">Kite</div>
                <div className="mt-1 text-xs text-muted-foreground">
                  {t('login.kubernetesDashboard')}
                </div>
              </div>
            </div>
            <div className="ml-auto flex items-center gap-2">
              <LanguageToggle />
              <ModeToggle />
            </div>
          </header>

          <main className="flex flex-1 items-center justify-center px-4 py-10 lg:px-6">
            <div className="mx-auto w-full max-w-lg">
              <div className="rounded-lg border bg-card p-8 text-center text-card-foreground shadow-sm">
                <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-md bg-primary/10 text-primary">
                  <MonitorSmartphone className="h-6 w-6" aria-hidden="true" />
                </div>
                <h1 className="mt-5 text-xl font-bold">
                  {t('login.sealosDesktopRequired.title')}
                </h1>
                <p className="mt-3 text-sm leading-6 text-muted-foreground">
                  {t('login.sealosDesktopRequired.description')}
                </p>
                <p className="mt-2 text-xs leading-5 text-muted-foreground">
                  {t('login.sealosDesktopRequired.hint')}
                </p>
                <Button
                  type="button"
                  variant="outline"
                  className="mt-6"
                  onClick={() => window.location.reload()}
                >
                  <RefreshCcw aria-hidden="true" />
                  {t('login.refresh')}
                </Button>
              </div>
            </div>
          </main>

          <Footer />
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-muted/20">
      <div className="flex min-h-screen flex-col">
        <header className="flex h-[var(--header-height)] shrink-0 items-center border-b bg-background px-4 lg:px-6">
          <div className="flex items-center gap-2">
            <img src={Logo} className="h-8 w-8 dark:invert" alt="Kite" />
            <div className="min-w-0">
              <div className="text-base font-semibold leading-none">Kite</div>
              <div className="mt-1 text-xs text-muted-foreground">
                {t('login.kubernetesDashboard')}
              </div>
            </div>
          </div>
          <div className="ml-auto flex items-center gap-2">
            <LanguageToggle />
            <ModeToggle />
          </div>
        </header>

        <main className="flex flex-1 items-center px-4 py-10 lg:px-6">
          <div className="mx-auto w-full max-w-4xl">
            <div className="flex flex-col gap-4 md:gap-6">
              <div>
                <h1 className="text-2xl font-bold">
                  {t('login.unavailableTitle')}
                </h1>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t('login.unavailableDescription')}
                </p>
              </div>

              <div className="rounded-lg border bg-card text-card-foreground shadow-sm">
                <div className="grid gap-0 lg:grid-cols-[minmax(0,1fr)_22rem]">
                  <div className="border-b p-5 lg:border-b-0 lg:border-r lg:p-6">
                    <div className="flex items-start gap-4">
                      <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-amber-50 text-amber-700 ring-1 ring-amber-200 dark:bg-amber-950/40 dark:text-amber-300 dark:ring-amber-900/60">
                        <AlertTriangle className="h-5 w-5" aria-hidden="true" />
                      </div>
                      <div className="min-w-0">
                        <div className="inline-flex rounded-md border border-amber-200 bg-amber-50 px-2 py-1 text-xs font-medium text-amber-900 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-100">
                          {t(`login.faultReasons.${reason}`)}
                        </div>
                        <h2 className="mt-4 text-lg font-semibold">
                          {t('login.actionRequiredTitle')}
                        </h2>
                        <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
                          {t('login.faultHint')}
                        </p>
                        {showSealosSdkStatus && (
                          <div className="mt-4 flex max-w-2xl items-start gap-3 rounded-md border bg-muted/30 p-3 text-left">
                            <div className="mt-0.5 shrink-0 text-muted-foreground">
                              {sealosSdkAccessStatus === 'checking' ? (
                                <Loader2
                                  className="h-4 w-4 animate-spin"
                                  aria-hidden="true"
                                />
                              ) : (
                                <Info className="h-4 w-4" aria-hidden="true" />
                              )}
                            </div>
                            <div className="min-w-0">
                              <div className="text-sm font-medium text-foreground">
                                {t(
                                  `login.sealosSdkStatus.${sdkStatusForKeys}.title`
                                )}
                              </div>
                              <div className="mt-1 text-xs leading-5 text-muted-foreground">
                                {t(
                                  `login.sealosSdkStatus.${sdkStatusForKeys}.description`
                                )}
                              </div>
                            </div>
                          </div>
                        )}
                        <Button
                          type="button"
                          variant="outline"
                          className="mt-5"
                          onClick={() => window.location.reload()}
                        >
                          <RefreshCcw aria-hidden="true" />
                          {t('login.refresh')}
                        </Button>
                      </div>
                    </div>
                  </div>

                  <div className="bg-muted/20 p-5 lg:p-6">
                    <div className="text-sm font-semibold text-foreground">
                      {t('login.operatorCheckTitle')}
                    </div>
                    <div className="mt-4 space-y-4">
                      {checks.map((item) => {
                        const Icon = item.icon
                        return (
                          <div key={item.label} className="flex gap-3">
                            <Icon
                              className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground"
                              aria-hidden="true"
                            />
                            <div>
                              <div className="text-sm font-medium text-foreground">
                                {item.label}
                              </div>
                              <div className="mt-0.5 text-xs leading-5 text-muted-foreground">
                                {item.description}
                              </div>
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </main>

        <Footer />
      </div>
    </div>
  )
}
