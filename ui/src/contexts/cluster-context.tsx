/* eslint-disable react-refresh/only-export-components */
import React, { createContext, useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { Cluster } from '@/types/api'
import {
  CURRENT_CLUSTER_CHANGE_EVENT,
  CURRENT_CLUSTER_STORAGE_KEY,
  readCurrentCluster,
  writeCurrentCluster,
} from '@/lib/current-cluster'
import { withSubPath } from '@/lib/subpath'

const isAsciiClusterName = (value: string) => /^[\x21-\x7E]+$/.test(value)

interface ClusterContextType {
  clusters: Cluster[]
  currentCluster: string | null
  currentClusterInfo: Cluster | null
  setCurrentCluster: (clusterName: string) => void
  isLoading: boolean
  isSwitching?: boolean
  error: Error | null
}

export const ClusterContext = createContext<ClusterContextType | undefined>(
  undefined
)

export const ClusterProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const getScopedNamespaceKey = (clusterName: string) =>
    `${clusterName}-scoped-namespace`
  const getSelectedNamespaceKey = (clusterName: string) =>
    `${clusterName}selectedNamespace`

  const [currentCluster, setCurrentClusterState] = useState<string | null>(
    readCurrentCluster()
  )
  const queryClient = useQueryClient()
  const [isSwitching, setIsSwitching] = useState(false)

  // Fetch clusters from API (this request shouldn't need cluster header)
  const {
    data: clusters = [],
    isLoading,
    error,
  } = useQuery<Cluster[]>({
    queryKey: ['clusters'],
    queryFn: async () => {
      const response = await fetch(withSubPath('/api/v1/clusters'), {
        cache: 'no-store',
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
        },
      })

      if (response.status === 403) {
        const errorData = await response.json().catch(() => ({}))
        const redirectUrl = response.headers.get('Location')
        if (redirectUrl) {
          window.location.href = redirectUrl
        }
        throw new Error(`${errorData.error || response.status}`)
      }

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(`${errorData.error || response.status}`)
      }

      return response.json()
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
  })

  const currentClusterInfo =
    clusters.find((cluster) => cluster.name === currentCluster) ?? null

  useEffect(() => {
    if (!currentClusterInfo || !currentClusterInfo.name) return

    const scopedNamespaceKey = getScopedNamespaceKey(currentClusterInfo.name)
    const selectedNamespaceKey = getSelectedNamespaceKey(
      currentClusterInfo.name
    )
    if (currentClusterInfo.namespaceScoped && currentClusterInfo.namespace) {
      localStorage.setItem(scopedNamespaceKey, currentClusterInfo.namespace)
      localStorage.setItem(selectedNamespaceKey, currentClusterInfo.namespace)
      return
    }
    localStorage.removeItem(scopedNamespaceKey)
    if (currentClusterInfo.namespace) {
      const selectedNamespace = localStorage.getItem(selectedNamespaceKey)
      if (!selectedNamespace) {
        localStorage.setItem(selectedNamespaceKey, currentClusterInfo.namespace)
      }
    }
  }, [currentClusterInfo])

  useEffect(() => {
    const syncFromStorage = () => {
      const clusterName = readCurrentCluster()
      if (clusterName === currentCluster) {
        return
      }
      setCurrentClusterState(clusterName)
    }

    const onClusterChange = () => syncFromStorage()
    const onStorage = (event: StorageEvent) => {
      if (event.key === null || event.key === CURRENT_CLUSTER_STORAGE_KEY) {
        syncFromStorage()
      }
    }

    window.addEventListener(
      CURRENT_CLUSTER_CHANGE_EVENT,
      onClusterChange as EventListener
    )
    window.addEventListener('storage', onStorage)

    return () => {
      window.removeEventListener(
        CURRENT_CLUSTER_CHANGE_EVENT,
        onClusterChange as EventListener
      )
      window.removeEventListener('storage', onStorage)
    }
  }, [currentCluster])

  // Set default cluster if none is selected
  useEffect(() => {
    if (clusters.length > 0 && !currentCluster) {
      const defaultCluster = clusters.find(
        (c) => c.isDefault && isAsciiClusterName(c.name)
      )
      const fallbackCluster = clusters.find((c) => isAsciiClusterName(c.name))
      if (defaultCluster) {
        setCurrentClusterState(defaultCluster.name)
        writeCurrentCluster(defaultCluster.name)
      } else if (fallbackCluster) {
        // If no default cluster, use the first one
        setCurrentClusterState(fallbackCluster.name)
        writeCurrentCluster(fallbackCluster.name)
      }
    }
    if (
      currentCluster &&
      clusters.length > 0 &&
      (!clusters.some((c) => c.name === currentCluster) ||
        !isAsciiClusterName(currentCluster))
    ) {
      // If current cluster is not in the list, reset it
      setCurrentClusterState(null)
      writeCurrentCluster(null)
    }
  }, [clusters, currentCluster])

  const setCurrentCluster = (clusterName: string) => {
    if (clusterName !== currentCluster && !isSwitching) {
      if (!isAsciiClusterName(clusterName)) {
        toast.error('Cluster name must use English/ASCII characters only')
        return
      }
      try {
        setIsSwitching(true)
        setCurrentClusterState(clusterName)
        writeCurrentCluster(clusterName)

        const selectedCluster = clusters.find(
          (cluster) => cluster.name === clusterName
        )
        if (selectedCluster?.namespaceScoped && selectedCluster.namespace) {
          localStorage.setItem(
            getScopedNamespaceKey(clusterName),
            selectedCluster.namespace
          )
          localStorage.setItem(
            getSelectedNamespaceKey(clusterName),
            selectedCluster.namespace
          )
        }

        setTimeout(async () => {
          await queryClient.invalidateQueries({
            predicate: (query) => {
              const key = query.queryKey[0] as string
              return !['user', 'auth', 'clusters'].includes(key)
            },
          })
          setIsSwitching(false)
          toast.success(`Switched to cluster: ${clusterName}`, {
            id: 'cluster-switch',
          })
        }, 300)
      } catch (error) {
        console.error('Failed to switch cluster:', error)
        setIsSwitching(false)
        toast.error('Failed to switch cluster', {
          id: 'cluster-switch',
        })
      }
    }
  }

  const value: ClusterContextType = {
    clusters,
    currentCluster,
    currentClusterInfo,
    setCurrentCluster,
    isLoading,
    isSwitching,
    error: error as Error | null,
  }

  return (
    <ClusterContext.Provider value={value}>{children}</ClusterContext.Provider>
  )
}
