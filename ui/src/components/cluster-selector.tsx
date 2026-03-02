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

const getClusterDisplayName = (cluster?: {
  name: string
  namespaceScoped?: boolean
  namespace?: string
}) => {
  if (!cluster) return 'Select Cluster'
  if (cluster.namespaceScoped && cluster.namespace) {
    return cluster.namespace
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
          className="flex items-center gap-2 h-8 px-3 max-w-full focus-visible:ring-0 focus-visible:border-transparent"
          disabled={isSwitching}
        >
          <IconServer className="h-4 w-4" />
          <span className="text-sm font-medium truncate">
            {isSwitching
              ? 'Switching...'
              : getClusterDisplayName(currentClusterData)}
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
                  {getClusterDisplayName(cluster)}
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
                  cluster.error ||
                    !isAsciiClusterName(cluster.name)
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
