/* eslint-disable react-refresh/only-export-components */
import {
  createContext,
  ReactNode,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react'
import i18n from '@/i18n'
import { useQueryClient } from '@tanstack/react-query'
import * as sealosDesktopSDK from 'sealos-desktop-sdk/app'

import { readAuthToken, writeAuthToken } from '@/lib/auth-token'
import {
  CURRENT_CLUSTER_CHANGE_EVENT,
  readCurrentCluster,
  writeCurrentCluster,
} from '@/lib/current-cluster'
import { withSubPath } from '@/lib/subpath'

interface UserCapabilities {
  aiEnabled?: boolean
  kubectlEnabled?: boolean
  canCreateCustomCRDGroup?: boolean
}

interface User {
  id: string
  username: string
  name: string
  avatar_url: string
  provider: string
  roles?: {
    name: string
    clusters?: string[]
    resources?: string[]
    namespaces?: string[]
    verbs?: string[]
  }[]
  sidebar_preference?: string
  capabilities?: UserCapabilities

  isAdmin(): boolean
}

interface AuthContextType {
  user: User | null
  isLoading: boolean
  sealosSdkAccessStatus: SealosSdkAccessStatus
  providers: string[]
  helmArtifactHubEnabled: boolean
  capabilities: Required<
    Pick<UserCapabilities, 'aiEnabled' | 'kubectlEnabled'>
  > &
    Pick<UserCapabilities, 'canCreateCustomCRDGroup'>
  login: (provider?: string) => Promise<void>
  loginWithPassword: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  checkAuth: () => Promise<void>
  refreshToken: () => Promise<void>
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

export function useAuth() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return context
}

interface AuthProviderProps {
  children: ReactNode
}

interface SealosSessionUser {
  k8s_username?: string
  name?: string
  avatar?: string
  nsid?: string
  ns_uid?: string
  userCrUid?: string
  userId?: string
  userUid?: string
}

interface SealosSession {
  token: string
  kubeconfig: string
  user?: SealosSessionUser
}

type SealosLanguage = 'en' | 'zh'
type SealosSdkAccessStatus =
  | 'disabled'
  | 'checking'
  | 'available'
  | 'unavailable'

interface SealosAppClient {
  getSession?: () => Promise<unknown>
  getLanguage?: () => Promise<unknown>
  addAppEventListen?: (
    name: string,
    fn: (eventData?: unknown) => unknown
  ) => (() => void) | undefined
}

interface SealosSessionResult {
  session: SealosSession | null
  sdkAccessible: boolean
}

const SEALOS_PROVIDER = 'sealos'
const SEALOS_LANGUAGE_CHANGED_EVENT = 'change_i18n'
const DEFAULT_CAPABILITIES = {
  aiEnabled: false,
  kubectlEnabled: false,
}

const getEnvFlag = (value: string | undefined): boolean | null => {
  if (value === 'true') return true
  if (value === 'false') return false
  return null
}

const shouldTrySealosAutoLogin = (): boolean => {
  const envFlag = getEnvFlag(import.meta.env.VITE_SEALOS_AUTO_LOGIN)
  if (envFlag !== null) return envFlag
  return true
}

const normalizeSealosSession = (raw: unknown): SealosSession | null => {
  if (typeof raw !== 'object' || raw === null) return null
  const value = raw as Record<string, unknown>
  const token = typeof value.token === 'string' ? value.token.trim() : ''
  const kubeconfig =
    typeof value.kubeconfig === 'string' ? value.kubeconfig.trim() : ''
  if (!token || !kubeconfig) return null
  const user =
    typeof value.user === 'object' && value.user !== null
      ? (value.user as SealosSessionUser)
      : undefined
  return { token, kubeconfig, user }
}

const normalizeSealosLanguage = (raw: unknown): SealosLanguage | null => {
  let language = ''

  if (typeof raw === 'string') {
    language = raw
  } else if (typeof raw === 'object' && raw !== null) {
    const value = raw as Record<string, unknown>
    language =
      (typeof value.lng === 'string' && value.lng) ||
      (typeof value.lang === 'string' && value.lang) ||
      (typeof value.language === 'string' && value.language) ||
      (typeof value.locale === 'string' && value.locale) ||
      ''
  }

  const normalized = language.trim().toLowerCase()
  if (normalized.startsWith('zh')) return 'zh'
  if (normalized.startsWith('en')) return 'en'
  return null
}

const withTimeout = async <T,>(
  promise: Promise<T>,
  timeoutMs: number
): Promise<T> =>
  await Promise.race([
    promise,
    new Promise<T>((_, reject) => {
      setTimeout(() => reject(new Error('Sealos session timeout')), timeoutMs)
    }),
  ])

const getSealosAppClient = (): SealosAppClient | null => {
  // Vite can pre-bundle this SDK subpath as CJS, so the live client may sit on the default export.
  const sdkModule = sealosDesktopSDK as unknown as {
    sealosApp?: SealosAppClient
    default?: {
      sealosApp?: SealosAppClient
    }
  }

  return sdkModule.sealosApp ?? sdkModule.default?.sealosApp ?? null
}

const getSealosSession = async (
  timeoutMs = 5000
): Promise<SealosSessionResult> => {
  let cleanup: (() => void) | undefined
  try {
    cleanup = sealosDesktopSDK.createSealosApp()
    const appClient = getSealosAppClient()
    if (typeof appClient?.getSession !== 'function') {
      return { session: null, sdkAccessible: false }
    }
    const rawSession = await withTimeout(appClient.getSession(), timeoutMs)
    return {
      session: normalizeSealosSession(rawSession),
      sdkAccessible: true,
    }
  } catch {
    return { session: null, sdkAccessible: false }
  } finally {
    if (typeof cleanup === 'function') {
      cleanup()
    }
  }
}

const getSealosLanguage = async (
  timeoutMs = 3000
): Promise<SealosLanguage | null> => {
  let cleanup: (() => void) | undefined
  try {
    cleanup = sealosDesktopSDK.createSealosApp()
    const appClient = getSealosAppClient()
    if (typeof appClient?.getLanguage !== 'function') {
      return null
    }
    const rawLanguage = await withTimeout(appClient.getLanguage(), timeoutMs)
    return normalizeSealosLanguage(rawLanguage)
  } catch {
    return null
  } finally {
    if (typeof cleanup === 'function') {
      cleanup()
    }
  }
}

export function AuthProvider({ children }: AuthProviderProps) {
  const [user, setUser] = useState<User | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [providers, setProviders] = useState<string[]>([])
  const [sealosAuthEnabled, setSealosAuthEnabled] = useState(false)
  const [helmArtifactHubEnabled, setHelmArtifactHubEnabled] = useState(true)
  const [sealosSdkAccessStatus, setSealosSdkAccessStatus] =
    useState<SealosSdkAccessStatus>('checking')
  const queryClient = useQueryClient()
  const userRef = useRef<User | null>(null)
  const sealosAuthEnabledRef = useRef(false)
  const sealosSyncPromiseRef = useRef<Promise<boolean> | null>(null)

  useEffect(() => {
    userRef.current = user
  }, [user])

  const buildAuthHeaders = (): HeadersInit => {
    const token = readAuthToken()
    if (!token) return {}
    return {
      Authorization: `Bearer ${token}`,
    }
  }

  const loadProviders = async (): Promise<boolean> => {
    try {
      const response = await fetch(withSubPath('/api/auth/providers'))
      if (response.ok) {
        const data = await response.json()
        setProviders(data.providers || [])
        setHelmArtifactHubEnabled(data.helm_artifact_hub_enabled !== false)
        const sealosEnabled = data.sealos_auth_enabled === true
        sealosAuthEnabledRef.current = sealosEnabled
        setSealosAuthEnabled(sealosEnabled)
        if (!sealosEnabled) {
          setSealosSdkAccessStatus('disabled')
        }
        return sealosEnabled
      }
    } catch (error) {
      console.error('Failed to load OAuth providers:', error)
    }
    setHelmArtifactHubEnabled(true)
    sealosAuthEnabledRef.current = false
    setSealosAuthEnabled(false)
    setSealosSdkAccessStatus('disabled')
    return false
  }

  const checkAuthInternal = useCallback(
    async (options?: {
      preserveUserOnFailure?: boolean
    }): Promise<User | null> => {
      const preserveUserOnFailure = options?.preserveUserOnFailure === true
      const previousUser = userRef.current

      try {
        const response = await fetch(withSubPath('/api/auth/user'), {
          credentials: 'include',
          headers: buildAuthHeaders(),
        })

        if (response.ok) {
          const data = await response.json()
          const user = data.user as User
          user.capabilities = data.capabilities as UserCapabilities | undefined
          user.isAdmin = function () {
            return (
              this.roles?.some(
                (role: { name: string }) => role.name === 'admin'
              ) || false
            )
          }
          setUser(user)
          return user
        }

        if (preserveUserOnFailure && previousUser) {
          return previousUser
        }
        writeAuthToken(null)
        setUser(null)
        return null
      } catch (error) {
        console.error('Auth check failed:', error)
        if (preserveUserOnFailure && previousUser) {
          return previousUser
        }
        writeAuthToken(null)
        setUser(null)
        return null
      }
    },
    []
  )

  const checkAuth = useCallback(async () => {
    await checkAuthInternal()
  }, [checkAuthInternal])

  const syncSealosSession = useCallback(
    async (
      currentUser: User | null,
      enabled = sealosAuthEnabledRef.current
    ): Promise<boolean> => {
      if (!enabled) {
        setSealosSdkAccessStatus('disabled')
        return false
      }
      if (!shouldTrySealosAutoLogin()) {
        setSealosSdkAccessStatus('disabled')
        return false
      }

      // Respect explicit non-sealos logins; only auto-sync when unauthenticated or already in sealos mode.
      if (currentUser && currentUser.provider !== SEALOS_PROVIDER) {
        setSealosSdkAccessStatus('disabled')
        return false
      }

      if (sealosSyncPromiseRef.current) {
        return await sealosSyncPromiseRef.current
      }

      const syncPromise = (async (): Promise<boolean> => {
        try {
          setSealosSdkAccessStatus('checking')
          const { session: sealosSession, sdkAccessible } =
            await getSealosSession()
          setSealosSdkAccessStatus(
            sdkAccessible ? 'available' : 'unavailable'
          )
          if (!sealosSession) {
            return false
          }

          const response = await fetch(withSubPath('/api/auth/login/sealos'), {
            method: 'POST',
            credentials: 'include',
            headers: {
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({
              token: sealosSession.token,
              kubeconfig: sealosSession.kubeconfig,
              user: sealosSession.user,
            }),
          })

          if (!response.ok) {
            return false
          }

          const data = await response.json()
          const accessToken =
            typeof data?.access_token === 'string'
              ? data.access_token.trim()
              : ''
          if (accessToken) {
            writeAuthToken(accessToken)
          } else if (data?.token_type === 'kite-cookie') {
            writeAuthToken(null)
          }
          await queryClient.invalidateQueries({ queryKey: ['clusters'] })
          await queryClient.invalidateQueries({ queryKey: ['cluster-list'] })
          await queryClient.refetchQueries({
            queryKey: ['clusters'],
            type: 'active',
          })

          const previousCluster = readCurrentCluster()
          const nextCluster =
            data?.cluster && typeof data.cluster === 'string'
              ? data.cluster
              : null

          if (nextCluster) {
            writeCurrentCluster(nextCluster)
          }

          if (nextCluster && nextCluster !== previousCluster) {
            await queryClient.invalidateQueries({
              predicate: (query) => {
                const key = query.queryKey[0] as string
                return !['user', 'auth', 'clusters', 'cluster-list'].includes(
                  key
                )
              },
              // Avoid immediate refetch with stale cluster header during workspace switch.
              refetchType: 'none',
            })
          }

          await checkAuthInternal({ preserveUserOnFailure: true })
          return true
        } catch (error) {
          console.error('Sealos session sync failed:', error)
          return false
        }
      })()

      sealosSyncPromiseRef.current = syncPromise
      try {
        return await syncPromise
      } finally {
        if (sealosSyncPromiseRef.current === syncPromise) {
          sealosSyncPromiseRef.current = null
        }
      }
    },
    [checkAuthInternal, queryClient]
  )

  const syncSealosLanguage = useCallback(
    async (currentUser: User | null, enabled = sealosAuthEnabledRef.current) => {
      if (!enabled) {
        return
      }
      if (!shouldTrySealosAutoLogin()) {
        return
      }

      // Respect explicit non-sealos logins; only auto-sync when unauthenticated or already in sealos mode.
      if (currentUser && currentUser.provider !== SEALOS_PROVIDER) {
        return
      }

      try {
        const sealosLanguage = await getSealosLanguage()
        if (!sealosLanguage) {
          return
        }

        const currentLanguage = (i18n.resolvedLanguage ?? i18n.language)
          .toLowerCase()
          .trim()
        if (currentLanguage.startsWith(sealosLanguage)) {
          return
        }

        await i18n.changeLanguage(sealosLanguage)
      } catch (error) {
        console.error('Sealos language sync failed:', error)
      }
    },
    []
  )

  useEffect(() => {
    if (!sealosAuthEnabled) {
      return
    }
    if (!shouldTrySealosAutoLogin()) {
      return
    }

    if (user && user.provider !== SEALOS_PROVIDER) {
      return
    }

    const cleanup = sealosDesktopSDK.createSealosApp()
    const appClient = getSealosAppClient()
    if (typeof appClient?.addAppEventListen !== 'function') {
      if (typeof cleanup === 'function') {
        cleanup()
      }
      return
    }
    const removeLanguageListener = appClient.addAppEventListen(
      SEALOS_LANGUAGE_CHANGED_EVENT,
      async (eventData?: unknown) => {
        const nextLanguage = normalizeSealosLanguage(eventData)
        if (nextLanguage) {
          const currentLanguage = (i18n.resolvedLanguage ?? i18n.language)
            .toLowerCase()
            .trim()
          if (!currentLanguage.startsWith(nextLanguage)) {
            await i18n.changeLanguage(nextLanguage)
          }
          return
        }

        // Fallback to querying current desktop language when event payload shape is unknown.
        void syncSealosLanguage(user)
      }
    )

    return () => {
      if (typeof removeLanguageListener === 'function') {
        removeLanguageListener()
      }
      if (typeof cleanup === 'function') {
        cleanup()
      }
    }
  }, [sealosAuthEnabled, syncSealosLanguage, user])

  const login = async (provider: string = 'github') => {
    try {
      const response = await fetch(
        withSubPath(`/api/auth/login?provider=${provider}`),
        {
          credentials: 'include',
        }
      )

      if (response.ok) {
        const data = await response.json()
        window.location.href = data.auth_url
      } else {
        throw new Error('Failed to initiate login')
      }
    } catch (error) {
      console.error('Login failed:', error)
      throw error
    }
  }

  const loginWithPassword = async (username: string, password: string) => {
    try {
      const response = await fetch(withSubPath('/api/auth/login/password'), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ username, password }),
        credentials: 'include',
      })

      if (response.ok) {
        writeAuthToken(null)
        await checkAuth()
      } else {
        const errorData = await response.json()
        throw new Error(errorData.error || 'Password login failed')
      }
    } catch (error) {
      console.error('Password login failed:', error)
      throw error
    }
  }

  const refreshToken = async () => {
    if (readAuthToken()) {
      return
    }

    try {
      const response = await fetch(withSubPath('/api/auth/refresh'), {
        method: 'POST',
        credentials: 'include',
        headers: buildAuthHeaders(),
      })

      if (!response.ok) {
        throw new Error('Failed to refresh token')
      }
    } catch (error) {
      console.error('Token refresh failed:', error)
      writeAuthToken(null)
      setUser(null)
      window.location.href = withSubPath('/login?reason=session_refresh_failed')
    }
  }

  const logout = async () => {
    try {
      const response = await fetch(withSubPath('/api/auth/logout'), {
        method: 'POST',
        credentials: 'include',
        headers: buildAuthHeaders(),
      })

      if (response.ok) {
        writeAuthToken(null)
        setUser(null)
        writeCurrentCluster(null)
        window.location.href = withSubPath('/login?reason=logout')
      } else {
        throw new Error('Failed to logout')
      }
    } catch (error) {
      console.error('Logout failed:', error)
      throw error
    }
  }

  useEffect(() => {
    const initAuth = async () => {
      setIsLoading(true)
      try {
        // loadProviders and checkAuthInternal hit independent endpoints and
        // write disjoint state, so they run in parallel; this removes one
        // full round-trip from the blocking auth gate on every page load.
        const [sealosEnabled, currentUser] = await Promise.all([
          loadProviders(),
          checkAuthInternal(),
        ])
        await Promise.all([
          syncSealosSession(currentUser, sealosEnabled),
          syncSealosLanguage(currentUser, sealosEnabled),
        ])
      } finally {
        setIsLoading(false)
      }
    }
    initAuth()
  }, [syncSealosLanguage, syncSealosSession])

  useEffect(() => {
    if (!sealosAuthEnabled) {
      return
    }
    if (user && user.provider !== SEALOS_PROVIDER) {
      return
    }

    const syncOnFocus = () => {
      if (document.visibilityState === 'hidden') {
        return
      }
      void syncSealosLanguage(user)
      void syncSealosSession(user)
    }

    window.addEventListener('focus', syncOnFocus)
    document.addEventListener('visibilitychange', syncOnFocus)

    return () => {
      window.removeEventListener('focus', syncOnFocus)
      document.removeEventListener('visibilitychange', syncOnFocus)
    }
  }, [sealosAuthEnabled, syncSealosLanguage, syncSealosSession, user])

  useEffect(() => {
    const syncPermissionsByCluster = () => {
      if (!user) return
      void checkAuthInternal({ preserveUserOnFailure: true })
    }
    window.addEventListener(
      CURRENT_CLUSTER_CHANGE_EVENT,
      syncPermissionsByCluster as EventListener
    )
    return () => {
      window.removeEventListener(
        CURRENT_CLUSTER_CHANGE_EVENT,
        syncPermissionsByCluster as EventListener
      )
    }
  }, [checkAuthInternal, user])

  // Set up automatic token refresh
  useEffect(() => {
    if (!user) return
    const refreshKey = 'lastRefreshTokenAt'
    const lastRefreshAt = localStorage.getItem(refreshKey)
    const now = Date.now()

    // If the last refresh was more than 30 minutes ago, refresh immediately
    if (!lastRefreshAt || now - Number(lastRefreshAt) > 30 * 60 * 1000) {
      refreshToken()
      localStorage.setItem(refreshKey, String(now))
    }

    const refreshInterval = setInterval(
      () => {
        refreshToken()
        localStorage.setItem(refreshKey, String(Date.now()))
      },
      30 * 60 * 1000
    ) // Refresh every 30 minutes

    return () => clearInterval(refreshInterval)
  }, [user])

  const value = {
    user,
    isLoading,
    sealosSdkAccessStatus,
    providers,
    helmArtifactHubEnabled,
    capabilities: {
      ...DEFAULT_CAPABILITIES,
      ...user?.capabilities,
    },
    login,
    loginWithPassword,
    logout,
    checkAuth,
    refreshToken,
  }

  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  )
}
