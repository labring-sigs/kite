export interface SealosSessionUser {
  k8s_username?: string
  name?: string
  avatar?: string
  nsid?: string
  ns_uid?: string
  userCrUid?: string
  userId?: string
  userUid?: string
}

export interface SealosSession {
  token: string
  kubeconfig: string
  user?: SealosSessionUser
}

interface SealosAppLike {
  getSession: () => Promise<unknown>
}

type CreateSealosAppLike = () => (() => void) | void

declare global {
  interface Window {
    sealosApp?: SealosAppLike
    createSealosApp?: CreateSealosAppLike
    __SEALOS_SESSION__?: unknown
  }
}

const getEnvFlag = (value: string | undefined): boolean | null => {
  if (value === 'true') return true
  if (value === 'false') return false
  return null
}

export const shouldTrySealosAutoLogin = (): boolean => {
  const envFlag = getEnvFlag(import.meta.env.VITE_SEALOS_AUTO_LOGIN)
  if (envFlag !== null) return envFlag
  try {
    return window.self !== window.top
  } catch {
    return true
  }
}

const isObject = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null

const normalizeSession = (raw: unknown): SealosSession | null => {
  if (!isObject(raw)) return null

  const token =
    typeof raw.token === 'string' ? raw.token.trim() : ''
  const kubeconfig =
    typeof raw.kubeconfig === 'string' ? raw.kubeconfig.trim() : ''
  if (!token || !kubeconfig) return null

  const user = isObject(raw.user)
    ? (raw.user as SealosSessionUser)
    : undefined

  return {
    token,
    kubeconfig,
    user,
  }
}

const withTimeout = async <T>(
  promise: Promise<T>,
  timeoutMs: number
): Promise<T> => {
  return await Promise.race([
    promise,
    new Promise<T>((_, reject) => {
      setTimeout(() => reject(new Error('Sealos session timeout')), timeoutMs)
    }),
  ])
}

const requestSessionFromParent = async (
  timeoutMs: number
): Promise<SealosSession | null> => {
  if (typeof window === 'undefined' || window.parent === window) return null

  const parentOrigin = import.meta.env.VITE_SEALOS_PARENT_ORIGIN || '*'

  return await new Promise<SealosSession | null>((resolve) => {
    const timer = setTimeout(() => {
      window.removeEventListener('message', onMessage)
      resolve(null)
    }, timeoutMs)

    const onMessage = (event: MessageEvent) => {
      if (parentOrigin !== '*' && event.origin !== parentOrigin) return
      if (!isObject(event.data)) return

      const type = event.data.type
      if (type !== 'kite:sealos-session' && type !== 'sealos:session') return

      const candidate = normalizeSession(
        event.data.session ?? event.data.payload ?? event.data
      )
      if (!candidate) return

      clearTimeout(timer)
      window.removeEventListener('message', onMessage)
      resolve(candidate)
    }

    window.addEventListener('message', onMessage)
    window.parent.postMessage(
      {
        type: 'kite:sealos-session-request',
        source: 'kite',
      },
      parentOrigin
    )
  })
}

export const getSealosSession = async (
  timeoutMs = 5000
): Promise<SealosSession | null> => {
  if (typeof window === 'undefined') return null

  const injected = normalizeSession(window.__SEALOS_SESSION__)
  if (injected) return injected

  const byParentMessage = await requestSessionFromParent(timeoutMs)
  if (byParentMessage) return byParentMessage

  if (!window.sealosApp || typeof window.sealosApp.getSession !== 'function') {
    return null
  }

  let cleanup: (() => void) | void = undefined
  try {
    if (typeof window.createSealosApp === 'function') {
      cleanup = window.createSealosApp()
    }
    const rawSession = await withTimeout(window.sealosApp.getSession(), timeoutMs)
    return normalizeSession(rawSession)
  } catch {
    return null
  } finally {
    if (typeof cleanup === 'function') {
      cleanup()
    }
  }
}
