import { useAuth } from '@/contexts/auth-context'
import { IconCheck, IconChevronDown, IconServer } from '@tabler/icons-react'

import { cn } from '@/lib/utils'
import { useCluster } from '@/hooks/use-cluster'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

const isAsciiClusterName = (value: string) => /^[\x21-\x7E]+$/.test(value)

const extractSealosWorkspace = (
  value: string | undefined,
  username: string | undefined
) => {
  if (!value || !username) return null
  if (!value.startsWith('sealos-') || !username.startsWith('sealos-')) {
    return null
  }

  const userPart = username.slice('sealos-'.length)
  if (!userPart) return null

  const prefix = `sealos-${userPart}-`
  if (!value.startsWith(prefix)) return null

  const workspace = value.slice(prefix.length).trim()
  return workspace || null
}

const getClusterDisplayName = (cluster?: {
  name: string
  namespaceScoped?: boolean
  namespace?: string
  username?: string
  provider?: string
}) => {
  if (!cluster) return 'Select Cluster'
  if (cluster.provider === 'sealos') {
    const sealosNamespace = extractSealosWorkspace(
      cluster.namespace,
      cluster.username
    )
    if (sealosNamespace) return sealosNamespace
  }
  if (cluster.namespaceScoped && cluster.namespace) {
    return cluster.namespace
  }
  if (cluster.provider === 'sealos') {
    const sealosNamespace = extractSealosWorkspace(
      cluster.name,
      cluster.username
    )
    if (sealosNamespace) return sealosNamespace
  }
  return cluster.name
}

export function ClusterSelector() {
  const {
    clusters,
    currentCluster,
    setCurrentCluster,
    isSwitching,
    isLoading,
  } = useCluster()
  const { user } = useAuth()

  if (isLoading || isSwitching) {
    return (
      <div className="flex items-center justify-center">
        <div className="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600" />
        {isSwitching && (
          <span className="ml-2 text-sm text-muted-foreground">
            Switching cluster...
          </span>
        )}
      </div>
    )
  }

  const currentClusterData = clusters.find((c) => c.name === currentCluster)

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="w-full justify-between h-8 px-3 focus-visible:ring-0 focus-visible:border-transparent"
          disabled={isSwitching}
        >
          <span className="flex items-center gap-2 min-w-0">
            <IconServer className="h-4 w-4 shrink-0" />
            <span className="text-sm font-medium truncate">
              {isSwitching
                ? 'Switching...'
                : currentClusterData
                  ? getClusterDisplayName({
                      ...currentClusterData,
                      username: user?.username,
                      provider: user?.provider,
                    })
                  : getClusterDisplayName()}
            </span>
          </span>
          <IconChevronDown className="h-3 w-3 opacity-50" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-60">
        {clusters.map((cluster) => (
          <DropdownMenuItem
            key={cluster.name}
            onClick={() => setCurrentCluster(cluster.name)}
            disabled={!!cluster.error || !isAsciiClusterName(cluster.name)}
            className="flex items-center justify-between"
          >
            <div className="flex flex-col overflow-hidden">
              <div className="flex items-center gap-2">
                <span className="font-medium">
                  {getClusterDisplayName({
                    ...cluster,
                    username: user?.username,
                    provider: user?.provider,
                  })}
                </span>
                {cluster.isDefault && (
                  <Badge className="text-xs">Default</Badge>
                )}
                {cluster.error && (
                  <Badge variant="destructive" className="text-xs">
                    Sync Error
                  </Badge>
                )}
                {!isAsciiClusterName(cluster.name) && (
                  <Badge variant="secondary" className="text-xs">
                    ASCII only
                  </Badge>
                )}
              </div>
              <span
                className={cn(
                  'text-xs truncate',
                  cluster.error || !isAsciiClusterName(cluster.name)
                    ? 'text-red-500'
                    : 'text-muted-foreground'
                )}
                title={cluster.error}
              >
                {cluster.error ||
                  (!isAsciiClusterName(cluster.name)
                    ? 'Please rename this cluster to English/ASCII'
                    : cluster.version)}
              </span>
            </div>
            {currentCluster === cluster.name && (
              <IconCheck className="h-4 w-4" />
            )}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
