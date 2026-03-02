/* eslint-disable react-refresh/only-export-components */
import {
  createContext,
  ReactNode,
  useContext,
  useEffect,
  useState,
} from 'react'
import { useQueryClient } from '@tanstack/react-query'
import * as sealosDesktopSDK from 'sealos-desktop-sdk/app'

import { withSubPath } from '@/lib/subpath'

interface User {
  id: string
  username: string
  name: string
  avatar_url: string
  provider: string
  roles?: { name: string }[]
  sidebar_preference?: string

  isAdmin(): boolean
}

interface AuthContextType {
  user: User | null
  isLoading: boolean
  providers: string[]
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

const getSealosSession = async (
  timeoutMs = 5000
): Promise<SealosSession | null> => {
  const cleanup = sealosDesktopSDK.createSealosApp()
  try {
    // NOTE: Reassign to sidestep the CJS interop quirk where the sealosApp value doesn’t update.
    const appClient = sealosDesktopSDK.sealosApp
    const rawSession = await withTimeout(appClient.getSession(), timeoutMs)
    return normalizeSealosSession(rawSession)
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
  const queryClient = useQueryClient()

  const loadProviders = async () => {
    try {
      const response = await fetch(withSubPath('/api/auth/providers'))
      if (response.ok) {
        const data = await response.json()
        setProviders(data.providers || [])
      }
    } catch (error) {
      console.error('Failed to load OAuth providers:', error)
    }
  }

  const checkAuthInternal = async (): Promise<User | null> => {
    try {
      const response = await fetch(withSubPath('/api/auth/user'), {
        credentials: 'include',
      })

      if (response.ok) {
        const data = await response.json()
        const user = data.user as User
        user.isAdmin = function () {
          return (
            this.roles?.some(
              (role: { name: string }) => role.name === 'admin'
            ) || false
          )
        }
        setUser(user)
        return user
      } else {
        setUser(null)
        return null
      }
    } catch (error) {
      console.error('Auth check failed:', error)
      setUser(null)
      return null
    }
  }

  const checkAuth = async () => {
    await checkAuthInternal()
  }

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
    try {
      const response = await fetch(withSubPath('/api/auth/refresh'), {
        method: 'POST',
        credentials: 'include',
      })

      if (!response.ok) {
        throw new Error('Failed to refresh token')
      }
    } catch (error) {
      console.error('Token refresh failed:', error)
      setUser(null)
      window.location.href = withSubPath('/login')
    }
  }

  const logout = async () => {
    try {
      const response = await fetch(withSubPath('/api/auth/logout'), {
        method: 'POST',
        credentials: 'include',
      })

      if (response.ok) {
        setUser(null)
        window.location.href = withSubPath('/login')
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
        await loadProviders()
        const currentUser = await checkAuthInternal()

        if (!currentUser && shouldTrySealosAutoLogin()) {
          const sealosSession = await getSealosSession()
          if (sealosSession) {
            const response = await fetch(
              withSubPath('/api/auth/login/sealos'),
              {
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
              }
            )

            if (response.ok) {
              const data = await response.json()
              if (data?.cluster && typeof data.cluster === 'string') {
                localStorage.setItem('current-cluster', data.cluster)
                document.cookie = `x-cluster-name=${data.cluster}; path=/`
              }
              await queryClient.invalidateQueries({ queryKey: ['init-check'] })
              await queryClient.invalidateQueries({ queryKey: ['clusters'] })
              await queryClient.invalidateQueries({
                queryKey: ['cluster-list'],
              })
              await checkAuthInternal()
            }
          }
        }
      } finally {
        setIsLoading(false)
      }
    }
    initAuth()
  }, [queryClient])

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
    providers,
    login,
    loginWithPassword,
    logout,
    checkAuth,
    refreshToken,
  }

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
