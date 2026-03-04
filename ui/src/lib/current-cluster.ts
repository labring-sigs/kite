export const CURRENT_CLUSTER_STORAGE_KEY = 'current-cluster'
export const CURRENT_CLUSTER_COOKIE_KEY = 'x-cluster-name'
export const CURRENT_CLUSTER_CHANGE_EVENT = 'kite:current-cluster-change'

export interface CurrentClusterChangeDetail {
  clusterName: string | null
}

export const readCurrentCluster = (): string | null => {
  return localStorage.getItem(CURRENT_CLUSTER_STORAGE_KEY)
}

export const writeCurrentCluster = (clusterName: string | null): void => {
  if (clusterName) {
    localStorage.setItem(CURRENT_CLUSTER_STORAGE_KEY, clusterName)
    document.cookie = `${CURRENT_CLUSTER_COOKIE_KEY}=${clusterName}; path=/`
  } else {
    localStorage.removeItem(CURRENT_CLUSTER_STORAGE_KEY)
    document.cookie = `${CURRENT_CLUSTER_COOKIE_KEY}=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT`
  }

  window.dispatchEvent(
    new CustomEvent<CurrentClusterChangeDetail>(CURRENT_CLUSTER_CHANGE_EVENT, {
      detail: { clusterName },
    })
  )
}
