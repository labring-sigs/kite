import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useOverview, useResourceUsageHistory } from '@/lib/api'
import { useCluster } from '@/hooks/use-cluster'
import NetworkUsageChart from '@/components/chart/network-usage-chart'
import ResourceUtilizationChart from '@/components/chart/resource-utilization'
import { ClusterStatsCards } from '@/components/cluster-stats-cards'
import { RecentEvents } from '@/components/recent-events'
import { ResourceCharts } from '@/components/resources-charts'

export function Overview() {
  const { t } = useTranslation()
  const [timeRange] = useState('30m')
  const { data: overview, isLoading, error, isError } = useOverview()
  const { currentClusterInfo } = useCluster()

  // The clusters list (loaded before this page renders) already knows
  // whether Prometheus is configured, so the usage-history query can start
  // in parallel with the overview request instead of waiting for it; the
  // overview flag remains as a fallback for older backends.
  const prometheusEnabled =
    currentClusterInfo?.prometheusEnabled ?? overview?.prometheusEnabled ?? false

  const {
    data: resourceUsage,
    isLoading: isLoadingResourceUsage,
    error: errorResourceUsage,
  } = useResourceUsageHistory(timeRange, {
    enabled: prometheusEnabled,
  })
  const resourceUsageError =
    resourceUsage && resourceUsage.cpu.length + resourceUsage.memory.length > 0
      ? null
      : errorResourceUsage

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center h-64 gap-2">
        <h2 className="text-lg font-semibold">{t('overview.failedToLoad')}</h2>
        <p className="text-sm text-muted-foreground">
          {error instanceof Error ? error.message : t('overview.unknownError')}
        </p>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold">{t('overview.title')}</h1>
      </div>

      <ClusterStatsCards stats={overview} isLoading={isLoading} />

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <ResourceCharts
          data={overview?.resource}
          isLoading={isLoading}
          error={error}
          isError={isError}
        />
        <RecentEvents />
      </div>

      {overview?.prometheusEnabled && (
        <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
          <ResourceUtilizationChart
            cpu={resourceUsage?.cpu || []}
            memory={resourceUsage?.memory || []}
            isLoading={isLoadingResourceUsage}
            error={resourceUsageError}
          />

          <NetworkUsageChart
            networkIn={resourceUsage?.networkIn || []}
            networkOut={resourceUsage?.networkOut || []}
            isLoading={isLoadingResourceUsage}
            error={resourceUsageError}
          />
        </div>
      )}
    </div>
  )
}
