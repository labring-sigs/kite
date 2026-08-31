// API types for Custom Resources

import {
  CustomResourceDefinition,
  CustomResourceDefinitionList,
} from 'kubernetes-types/apiextensions/v1'
import {
  DaemonSet,
  DaemonSetList,
  Deployment,
  DeploymentList,
  ReplicaSet,
  ReplicaSetList,
  StatefulSet,
  StatefulSetList,
} from 'kubernetes-types/apps/v1'
import {
  HorizontalPodAutoscaler,
  HorizontalPodAutoscalerList,
} from 'kubernetes-types/autoscaling/v2'
import { CronJob, CronJobList, Job, JobList } from 'kubernetes-types/batch/v1'
import {
  ConfigMap,
  ConfigMapList,
  Event,
  EventList,
  Namespace,
  NamespaceList,
  Node,
  PersistentVolume,
  PersistentVolumeClaim,
  PersistentVolumeClaimList,
  PersistentVolumeList,
  Pod,
  Secret,
  SecretList,
  Service,
  ServiceAccount,
  ServiceAccountList,
  ServiceList,
} from 'kubernetes-types/core/v1'
import { Ingress, IngressList } from 'kubernetes-types/networking/v1'
import {
  ClusterRole,
  ClusterRoleBinding,
  ClusterRoleBindingList,
  ClusterRoleList,
  Role as RawRole,
  RoleBinding,
  RoleBindingList,
  RoleList,
} from 'kubernetes-types/rbac/v1'
import { StorageClass, StorageClassList } from 'kubernetes-types/storage/v1'

export interface CustomResource {
  apiVersion: string
  kind: string
  metadata: {
    name: string
    namespace?: string
    creationTimestamp: string
    uid?: string
    resourceVersion?: string
    labels?: Record<string, string>
    annotations?: Record<string, string>
  }
  spec?: Record<string, unknown>
  status?: Record<string, unknown>
}

export interface CustomResourceList {
  apiVersion: string
  kind: string
  items: CustomResource[]
  metadata?: {
    continue?: string
    remainingItemCount?: number
  }
}

export interface DeploymentRelatedResource {
  events: Event[]
  pods: Pod[]
  services: Service[]
}

export interface HelmReleaseResource {
  apiVersion: string
  kind: string
  name: string
  namespace?: string
}

export interface HelmReleaseHistoryItem {
  revision: number
  status: string
  chart: string
  chartName: string
  chartVersion: string
  appVersion?: string
  values?: Record<string, unknown>
  description?: string
  firstDeployed?: string
  lastDeployed?: string
  deleted?: string
}

export interface HelmReleaseHistoryResponse {
  items: HelmReleaseHistoryItem[]
}

export interface HelmRelease {
  apiVersion: 'v1'
  kind: 'HelmRelease'
  metadata: {
    name: string
    namespace: string
    uid?: string
    resourceVersion?: string
    creationTimestamp?: string
    labels?: Record<string, string>
    annotations?: Record<string, string>
  }
  spec: {
    releaseName: string
    namespace: string
    chart: string
    chartName: string
    chartVersion: string
    appVersion?: string
    icon?: string
    revision: number
    values?: Record<string, unknown>
    defaultValues?: Record<string, unknown>
    manifest?: string
    notes?: string
    description?: string
  }
  status: {
    status: string
    firstDeployed?: string
    lastDeployed?: string
    deleted?: string
    resources?: HelmReleaseResource[]
  }
}

export interface HelmReleaseList {
  apiVersion: 'v1'
  kind: 'HelmReleaseList'
  items: HelmRelease[]
  metadata?: listMetadataType
}

export interface HelmRepository {
  id: number
  name: string
  url: string
  username?: string
  hasAuth: boolean
  createdAt: string
  updatedAt: string
}

export interface RepositoryUploadTargetConfig {
  configured: boolean
  maxBytes: number
}

export interface OCIChartUploadConfig extends RepositoryUploadTargetConfig {
  registryBase?: string
  repositoryName?: string
}

export interface ContainerImageUploadConfig extends RepositoryUploadTargetConfig {
  registry?: string
  repositoryPrefix?: string
}

export interface RepositoryUploadConfig {
  chart: OCIChartUploadConfig
  image: ContainerImageUploadConfig
}

export interface OCIChartUploadResult {
  repositoryName: string
  chartName: string
  version: string
  chartUrl: string
  pushedRef: string
  digest?: string
  size: number
}

export interface ContainerImageUploadResult {
  imageRef: string
  digest?: string
  size: number
}

export interface OfflineBundleExportApplication {
  repositoryName: string
  chartName: string
  version: string
  values?: Record<string, unknown>
}

export interface OfflineBundleImportResult {
  apps: OfflineBundleImportAppResult[]
}

export type OfflineBundleImportJobStatus =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'

export interface OfflineBundleImportJob {
  id: string
  status: OfflineBundleImportJobStatus
  createdAt: string
  updatedAt: string
  completedAt?: string
  result?: OfflineBundleImportResult
  error?: string
}

export interface OfflineBundleImportAppResult {
  name: string
  version: string
  chartUrl?: string
  images?: ContainerImageUploadResult[]
  chart?: OCIChartUploadResult
  skipped?: boolean
  error?: string
}

export type HelmChartSource = 'repository' | 'artifacthub' | 'oci'

export interface HelmChart {
  repositoryId: number
  repositoryName: string
  repositoryUrl: string
  source?: HelmChartSource
  name: string
  version: string
  appVersion?: string
  kubeVersion?: string
  description?: string
  icon?: string
  home?: string
  artifactHubUrl?: string
  chartUrl?: string
  sources?: string[]
  keywords?: string[]
  maintainers?: {
    name: string
    email?: string
    url?: string
  }[]
  deprecated?: boolean
  updatedAt?: string
}

export interface HelmChartVersion {
  version: string
  appVersion?: string
  publishedAt?: string
}

export interface HelmChartList {
  items: HelmChart[]
  total?: number
}

export type HelmChartContentType = 'values' | 'templates'

export interface HelmChartTemplate {
  path: string
  content: string
}

export interface HelmChartContent {
  content?: string
  templates?: HelmChartTemplate[]
}

export interface HelmChartDetail extends HelmChart {
  readme?: string
  versions: HelmChartVersion[]
}

export interface HelmReleaseInstallRequest {
  releaseName: string
  namespace?: string
  chartUrl: string
  chartName?: string
  chartVersion?: string
  repositoryName?: string
  source?: HelmChartSource
  values?: Record<string, unknown>
  description?: string
  createNamespace?: boolean
  wait?: boolean
}

export interface HelmReleaseUpgradeRequest {
  chartUrl?: string
  chartVersion?: string
  repositoryName?: string
  source?: HelmChartSource
  values?: Record<string, unknown>
  description?: string
  forceConflicts?: boolean
  wait?: boolean
  rollbackOnFailure?: boolean
}

export interface HelmReleaseAutoUpgrade {
  clusterName: string
  namespace: string
  releaseName: string
  enabled: boolean
  scheduleType: 'interval' | 'daily'
  intervalMinutes: number
  scheduleTime: string
  timeoutMinutes: number
  rollbackOnFailure: boolean
  source?: HelmChartSource
  repositoryName?: string
  chartName?: string
  lastCheckedAt?: string
  lastUpgradedAt?: string
  lastError?: string
}

export interface HelmReleaseAutoUpgradeRequest {
  enabled: boolean
  scheduleType: 'interval' | 'daily'
  intervalMinutes: number
  scheduleTime: string
  timeoutMinutes: number
  rollbackOnFailure: boolean
  source?: HelmChartSource
  repositoryName?: string
  chartName?: string
}

export interface HelmReleaseDryRunResource {
  path: string
  content: string
  originalContent?: string
  modifiedContent?: string
  status?: 'added' | 'deleted' | 'changed' | 'unchanged'
  apiVersion?: string
  kind?: string
  name?: string
  namespace?: string
}

export interface HelmReleaseImageCheck {
  enabled: boolean
  registry?: string
  allImages?: string[]
  externalImages?: string[]
  injectedValues?: boolean
}

export interface HelmReleaseDryRunResponse {
  resources: HelmReleaseDryRunResource[]
  imageCheck?: HelmReleaseImageCheck
}

// Resource type definitions
export type ResourceType =
  | 'pods'
  | 'deployments'
  | 'statefulsets'
  | 'daemonsets'
  | 'jobs'
  | 'cronjobs'
  | 'services'
  | 'configmaps'
  | 'secrets'
  | 'ingresses'
  | 'gateways'
  | 'httproutes'
  | 'namespaces'
  | 'crds'
  | 'crs'
  | 'nodes'
  | 'events'
  | 'persistentvolumes'
  | 'persistentvolumeclaims'
  | 'storageclasses'
  | 'podmetrics'
  | 'replicasets'
  | 'serviceaccounts'
  | 'roles'
  | 'rolebindings'
  | 'clusterroles'
  | 'clusterrolebindings'
  | 'horizontalpodautoscalers'
  | 'helmreleases'

export const clusterScopeResources: ResourceType[] = [
  'crds',
  'namespaces',
  'persistentvolumes',
  'nodes',
  'storageclasses',
  'clusterroles',
  'clusterrolebindings',
]

type listMetadataType = {
  continue?: string
  remainingItemCount?: number
}

// Define resource type mappings
export interface ResourcesTypeMap {
  pods: {
    items: PodWithMetrics[]
    metadata?: listMetadataType
  }
  deployments: DeploymentList
  statefulsets: StatefulSetList
  daemonsets: DaemonSetList
  jobs: JobList
  cronjobs: CronJobList
  services: ServiceList
  configmaps: ConfigMapList
  secrets: SecretList
  persistentvolumeclaims: PersistentVolumeClaimList
  ingresses: IngressList
  gateways: CustomResourceList
  httproutes: CustomResourceList
  namespaces: NamespaceList
  crds: CustomResourceDefinitionList
  crs: {
    items: CustomResource[]
    metadata?: listMetadataType
  }
  nodes: {
    items: NodeWithMetrics[]
    metadata?: listMetadataType
  }
  events: EventList
  persistentvolumes: PersistentVolumeList
  storageclasses: StorageClassList
  podmetrics: {
    items: PodMetrics[]
    metadata?: listMetadataType
  }
  replicasets: ReplicaSetList
  serviceaccounts: ServiceAccountList
  roles: RoleList
  rolebindings: RoleBindingList
  clusterroles: ClusterRoleList
  clusterrolebindings: ClusterRoleBindingList
  horizontalpodautoscalers: HorizontalPodAutoscalerList
  helmreleases: HelmReleaseList
}

export interface PodMetrics {
  metadata: {
    name: string
    namespace: string
    labels?: Record<string, string>
    annotations?: Record<string, string>
    creationTimestamp?: string
    uid?: string
    resourceVersion?: string
  }
  containers: {
    name: string // container name
    usage: {
      cpu: string // 214572390n
      memory: string // 2956516Ki
    }
  }[]
}

export type MetricsData = {
  cpuUsage?: number
  memoryUsage?: number
  cpuLimit?: number
  memoryLimit?: number
  cpuRequest?: number
  memoryRequest?: number
  pods?: number
  podsLimit?: number
}

export type PodWithMetrics = Pod & {
  metrics?: MetricsData
}

export type NodeWithMetrics = Node & {
  metrics?: MetricsData
}

export interface ResourceTypeMap {
  pods: PodWithMetrics
  deployments: Deployment
  statefulsets: StatefulSet
  daemonsets: DaemonSet
  jobs: Job
  cronjobs: CronJob
  services: Service
  configmaps: ConfigMap
  secrets: Secret
  persistentvolumeclaims: PersistentVolumeClaim
  ingresses: Ingress
  gateways: CustomResource
  httproutes: CustomResource
  namespaces: Namespace
  crds: CustomResourceDefinition
  crs: CustomResource
  nodes: NodeWithMetrics
  events: Event
  persistentvolumes: PersistentVolume
  storageclasses: StorageClass
  replicasets: ReplicaSet
  podmetrics: PodMetrics
  serviceaccounts: ServiceAccount
  roles: RawRole
  rolebindings: RoleBinding
  clusterroles: ClusterRole
  clusterrolebindings: ClusterRoleBinding
  horizontalpodautoscalers: HorizontalPodAutoscaler
  helmreleases: HelmRelease
}

export interface RecentEvent {
  type: string
  reason: string
  message: string
  involvedObjectKind: string
  involvedObjectName: string
  namespace?: string
  timestamp: string
}

export interface UsageDataPoint {
  timestamp: string
  value: number
}

export interface ResourceUsageHistory {
  cpu: UsageDataPoint[]
  memory: UsageDataPoint[]
  networkIn: UsageDataPoint[]
  networkOut: UsageDataPoint[]
  diskRead: UsageDataPoint[]
  diskWrite: UsageDataPoint[]
  namespace?: string
  cpuUtilizationMode?: 'cluster_capacity' | 'namespace_quota'
  memoryUtilizationMode?: 'cluster_capacity' | 'namespace_quota'
}

// Pod monitoring types
export interface PodMetrics {
  cpu: UsageDataPoint[]
  memory: UsageDataPoint[]
  networkIn?: UsageDataPoint[]
  networkOut?: UsageDataPoint[]
  diskRead?: UsageDataPoint[]
  diskWrite?: UsageDataPoint[]
  fallback?: boolean
}

export interface OverviewData {
  totalNodes: number
  readyNodes: number
  totalPods: number
  runningPods: number
  totalNamespaces: number
  totalIngresses: number
  totalPVCs: number
  totalServices: number
  prometheusEnabled: boolean
  resource: {
    cpu: {
      allocatable: number
      requested: number
      limited: number
      basis?: 'cluster_allocatable' | 'namespace_quota' | 'namespace_no_quota'
    }
    memory: {
      allocatable: number
      requested: number
      limited: number
      basis?: 'cluster_allocatable' | 'namespace_quota' | 'namespace_no_quota'
    }
  }
}

// Pagination types
export interface PaginationInfo {
  hasNextPage: boolean
  nextContinueToken?: string
  remainingItems?: number
}

export interface PaginationOptions {
  limit?: number
  continueToken?: string
}

// Pod current metrics types
export interface PodCurrentMetrics {
  podName: string
  namespace: string
  cpu: number // CPU cores
  memory: number // Memory in MB
}

export interface ImageTagInfo {
  name: string
  timestamp?: string
}

export interface RelatedResources {
  type: string
  name: string
  namespace?: string
  apiVersion?: string
}

export interface Cluster {
  id: number
  name: string
  description?: string
  version?: string
  config?: string
  enabled: boolean
  inCluster: boolean
  isDefault: boolean
  createdAt: string
  updatedAt: string
  prometheusURL?: string
  namespaceScoped?: boolean
  namespace?: string
  // Surfaced by /api/v1/clusters so the dashboard can start Prometheus
  // queries before the overview response arrives.
  prometheusEnabled?: boolean
  error?: string
}

export interface OAuthProvider {
  id: number
  name: string
  clientId: string
  clientSecret: string
  authUrl?: string
  tokenUrl?: string
  userInfoUrl?: string
  scopes?: string
  issuer?: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface RoleAssignment {
  id: number
  roleId: number
  subjectType: 'user' | 'group'
  subject: string
  createdAt: string
  updatedAt: string
}

export interface Role {
  id: number
  name: string
  description?: string
  isSystem?: boolean
  clusters: string[]
  namespaces: string[]
  resources: string[]
  verbs: string[]
  assignments?: RoleAssignment[]
  createdAt: string
  updatedAt: string
}

export interface UserItem {
  id: number
  username: string
  provider: string
  createdAt: string
  lastLoginAt?: string
  enabled?: boolean
  avatar_url?: string
  name?: string
  roles?: Role[]
}

export interface FetchUserListResponse {
  users: UserItem[]
  total: number
  page: number
  size: number
}

export interface APIKey {
  id: number
  username: string
  apiKey: string
  lastLoginAt?: string
  createdAt: string
  updatedAt: string
  roles?: Role[]
}

export interface GeneralSetting {
  aiAgentEnabled: boolean
  aiProvider: 'openai' | 'anthropic'
  aiModel: string
  aiApiKey: string
  aiApiKeyConfigured: boolean
  aiBaseUrl: string
  aiMaxTokens: number
  kubectlEnabled: boolean
  kubectlImage: string
  nodeTerminalImage: string
  enableAnalytics: boolean
  enableVersionCheck: boolean
  passwordLoginDisabled: boolean
  loginPrompt: string
}

export interface GeneralSettingUpdateRequest {
  aiAgentEnabled: boolean
  aiProvider: 'openai' | 'anthropic'
  aiModel: string
  aiApiKey?: string
  aiBaseUrl: string
  aiMaxTokens: number
  kubectlEnabled: boolean
  kubectlImage: string
  nodeTerminalImage: string
  enableAnalytics: boolean
  enableVersionCheck: boolean
  passwordLoginDisabled?: boolean
  loginPrompt: string
}

// Resource History types
export interface ResourceHistory {
  id: number
  clusterName: string
  resourceType: string
  resourceName: string
  namespace: string
  operationType: string
  resourceYaml: string
  previousYaml: string
  success: boolean
  errorMessage: string
  operatorId: number
  operator: {
    username: string
    provider: string
  }
  createdAt: string
  updatedAt: string
}

export interface ResourceHistoryResponse {
  data: ResourceHistory[]
  pagination: {
    page: number
    pageSize: number
    total: number
    totalPages: number
    hasNextPage: boolean
    hasPrevPage: boolean
  }
}

export interface AuditLogResponse {
  data: ResourceHistory[]
  total: number
  page: number
  size: number
}
export interface ResourceTemplate {
  ID: number
  name: string
  description: string
  yaml: string
}
