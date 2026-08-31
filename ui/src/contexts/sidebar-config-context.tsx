/* eslint-disable react-refresh/only-export-components */
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import * as React from 'react'
import {
  Icon,
  IconArrowsHorizontal,
  IconBell,
  IconBox,
  IconBoxMultiple,
  IconClockHour4,
  IconCode,
  IconDatabase,
  IconFileDatabase,
  IconKey,
  IconLoadBalancer,
  IconLock,
  IconMap,
  IconNetwork,
  IconPackage,
  IconPlayerPlay,
  IconProps,
  IconRocket,
  IconRoute,
  IconRouter,
  IconServer2,
  IconShield,
  IconShieldCheck,
  IconStack2,
  IconTopologyBus,
  IconUser,
  IconUsers,
} from '@tabler/icons-react'

import {
  DefaultMenus,
  SidebarConfig,
  SidebarGroup,
  SidebarItem,
} from '@/types/sidebar'
import { useBuiltinSidebarCRDs } from '@/lib/api'
import { isSidebarPathHidden } from '@/lib/resource-visibility'
import { withSubPath } from '@/lib/subpath'

import { useAuth } from './auth-context'

const iconMap = {
  IconBox,
  IconRocket,
  IconStack2,
  IconTopologyBus,
  IconPlayerPlay,
  IconClockHour4,
  IconRouter,
  IconNetwork,
  IconLoadBalancer,
  IconRoute,
  IconFileDatabase,
  IconDatabase,
  IconMap,
  IconLock,
  IconUser,
  IconShield,
  IconUsers,
  IconShieldCheck,
  IconKey,
  IconBoxMultiple,
  IconServer2,
  IconBell,
  IconCode,
  IconArrowsHorizontal,
  IconPackage,
}

const getIconName = (iconComponent: React.ComponentType): string => {
  const entry = Object.entries(iconMap).find(
    ([, component]) => component === iconComponent
  )
  return entry ? entry[0] : 'IconBox'
}

interface SidebarConfigContextType {
  config: SidebarConfig | null
  isLoading: boolean
  hasUpdate: boolean
  canCreateCustomCRDGroup: boolean
  shouldShowSidebarItem: (groupId: string, item: SidebarItem) => boolean
  resolveSidebarItemTitle: (groupId: string, item: SidebarItem) => string
  getSidebarItemScope: (
    groupId: string,
    item: SidebarItem
  ) => 'Cluster' | 'Namespaced' | string | undefined
  updateConfig: (updates: Partial<SidebarConfig>) => void
  toggleItemVisibility: (itemId: string) => void
  toggleGroupVisibility: (groupId: string) => void
  toggleItemPin: (itemId: string) => void
  toggleGroupCollapse: (groupId: string) => void
  resetConfig: () => void
  getIconComponent: (
    iconName: string
  ) =>
    | React.ForwardRefExoticComponent<IconProps & React.RefAttributes<Icon>>
    | React.ElementType
  createCustomGroup: (groupName: string) => void
  addCRDToGroup: (groupId: string, crdName: string, kind: string) => void
  removeCRDToGroup: (groupId: string, crdName: string) => void
  removeCustomGroup: (groupId: string) => void
  moveGroup: (groupId: string, direction: 'up' | 'down') => void
}

const SidebarConfigContext = createContext<
  SidebarConfigContextType | undefined
>(undefined)

export const useSidebarConfig = () => {
  const context = useContext(SidebarConfigContext)
  if (!context) {
    throw new Error(
      'useSidebarConfig must be used within a SidebarConfigProvider'
    )
  }
  return context
}

interface SidebarConfigProviderProps {
  children: React.ReactNode
}

const getSidebarGroupID = (groupKey: string): string =>
  groupKey.toLowerCase().replace(/\./g, '-').replace(/\s+/g, '-')

const BUILTIN_CR_GROUP_KEY = 'sidebar.groups.cr'
const BUILTIN_CR_GROUP_ID = getSidebarGroupID(BUILTIN_CR_GROUP_KEY)
const APPLICATION_GROUP_KEY = 'sidebar.groups.application'
const APPLICATION_GROUP_ID = getSidebarGroupID(APPLICATION_GROUP_KEY)
// Candidate builtin CR sidebar entries. An entry is only shown when the
// backend's builtin-CRD discovery also returns it (installed on the cluster
// and readable by the caller), so admin-only CRDs such as the Cilium egress
// gateway policy stay hidden for namespace-scoped workspace users.
const BUILTIN_CRD_NAMES = [
  'apps.app.sealos.io',
  'devboxes.devbox.sealos.io',
  'rabbitmqclusters.rabbitmq.com',
  'elasticsearches.elasticsearch.k8s.elastic.co',
  'clusters.apps.kubeblocks.io',
  'sealosnetworkpolicies.networking.sealos.io',
  'ciliumegressgatewaypolicies.cilium.io',
] as const
const BUILTIN_CRD_NAME_SET = new Set<string>(BUILTIN_CRD_NAMES)

const buildBuiltinCRSidebarItems = (): SidebarItem[] =>
  BUILTIN_CRD_NAMES.map((crdName, index) => ({
    id: `${BUILTIN_CR_GROUP_ID}-${`/crds/${crdName}`.replace(/[^a-zA-Z0-9]/g, '-')}`,
    titleKey: crdName,
    url: `/crds/${crdName}`,
    icon: 'IconCode',
    visible: true,
    pinned: false,
    order: index,
  }))

const buildApplicationSidebarItems = (): SidebarItem[] =>
  [
    {
      titleKey: 'nav.helmReleases',
      url: '/helmreleases',
      icon: 'IconPackage',
    },
    {
      titleKey: 'nav.helmCharts',
      url: '/charts',
      icon: 'IconPackage',
    },
  ].map((item, index) => ({
    id: `${APPLICATION_GROUP_ID}-${item.url.replace(/[^a-zA-Z0-9]/g, '-')}`,
    titleKey: item.titleKey,
    url: item.url,
    icon: item.icon,
    visible: true,
    pinned: false,
    order: index,
  }))

const ensureApplicationGroup = (config: SidebarConfig): SidebarConfig => {
  const applicationItems = buildApplicationSidebarItems()
  const existingGroupIndex = config.groups.findIndex(
    (group) => group.id === APPLICATION_GROUP_ID
  )

  if (existingGroupIndex === -1) {
    const groups = [
      {
        id: APPLICATION_GROUP_ID,
        nameKey: APPLICATION_GROUP_KEY,
        items: applicationItems,
        visible: true,
        collapsed: false,
        order: 0,
      },
      ...config.groups,
    ].map((group, index) => ({ ...group, order: index }))

    return {
      ...config,
      groups,
      groupOrder: groups.map((group) => group.id),
    }
  }

  const groups = config.groups.map((group) => {
    if (group.id !== APPLICATION_GROUP_ID) {
      return group
    }

    const existingItemIDs = new Set(group.items.map((item) => item.id))
    const missingItems = applicationItems.filter(
      (item) => !existingItemIDs.has(item.id)
    )

    if (missingItems.length === 0 && group.nameKey === APPLICATION_GROUP_KEY) {
      return group
    }

    return {
      ...group,
      nameKey: APPLICATION_GROUP_KEY,
      items:
        missingItems.length === 0
          ? group.items
          : [
              ...group.items,
              ...missingItems.map((item, index) => ({
                ...item,
                order: group.items.length + index,
              })),
            ],
    }
  })

  return {
    ...config,
    groups,
    groupOrder: config.groupOrder.includes(APPLICATION_GROUP_ID)
      ? config.groupOrder
      : [APPLICATION_GROUP_ID, ...config.groupOrder],
  }
}

const ensureBuiltinCRGroup = (config: SidebarConfig): SidebarConfig => {
  const builtinItems = buildBuiltinCRSidebarItems()
  const existingGroupIndex = config.groups.findIndex(
    (group) => group.id === BUILTIN_CR_GROUP_ID
  )

  if (existingGroupIndex === -1) {
    const maxOrder = config.groups.reduce(
      (maxOrderSoFar, group) => Math.max(maxOrderSoFar, group.order),
      -1
    )
    const builtinGroup: SidebarGroup = {
      id: BUILTIN_CR_GROUP_ID,
      nameKey: BUILTIN_CR_GROUP_KEY,
      items: builtinItems,
      visible: true,
      collapsed: false,
      order: maxOrder + 1,
    }

    return {
      ...config,
      groups: [...config.groups, builtinGroup],
      groupOrder: config.groupOrder.includes(BUILTIN_CR_GROUP_ID)
        ? config.groupOrder
        : [...config.groupOrder, BUILTIN_CR_GROUP_ID],
    }
  }

  const groups = config.groups.map((group) => {
    if (group.id !== BUILTIN_CR_GROUP_ID) {
      return group
    }

    const existingItemIDs = new Set(group.items.map((item) => item.id))
    const missingItems = builtinItems.filter(
      (item) => !existingItemIDs.has(item.id)
    )

    if (missingItems.length === 0 && group.nameKey === BUILTIN_CR_GROUP_KEY) {
      return group
    }

    return {
      ...group,
      nameKey: BUILTIN_CR_GROUP_KEY,
      items:
        missingItems.length === 0
          ? group.items
          : [
              ...group.items,
              ...missingItems.map((item, index) => ({
                ...item,
                order: group.items.length + index,
              })),
            ],
    }
  })

  return {
    ...config,
    groups,
    groupOrder: config.groupOrder.includes(BUILTIN_CR_GROUP_ID)
      ? config.groupOrder
      : [...config.groupOrder, BUILTIN_CR_GROUP_ID],
  }
}

const getCRDNameFromSidebarURL = (url: string): string | null => {
  const [resource, crdName] = url.replace(/^\/+/, '').split('/')
  if (resource !== 'crds' || !crdName) {
    return null
  }
  return crdName
}

const defaultMenus: DefaultMenus = {
  [APPLICATION_GROUP_KEY]: [
    { titleKey: 'nav.helmReleases', url: '/helmreleases', icon: IconPackage },
    { titleKey: 'nav.helmCharts', url: '/charts', icon: IconPackage },
  ],
  'sidebar.groups.workloads': [
    { titleKey: 'nav.pods', url: '/pods', icon: IconBox },
    { titleKey: 'nav.deployments', url: '/deployments', icon: IconRocket },
    {
      titleKey: 'nav.statefulsets',
      url: '/statefulsets',
      icon: IconStack2,
    },
    {
      titleKey: 'nav.daemonsets',
      url: '/daemonsets',
      icon: IconTopologyBus,
    },
    { titleKey: 'nav.jobs', url: '/jobs', icon: IconPlayerPlay },
    { titleKey: 'nav.cronjobs', url: '/cronjobs', icon: IconClockHour4 },
  ],
  'sidebar.groups.traffic': [
    { titleKey: 'nav.ingresses', url: '/ingresses', icon: IconRouter },
    { titleKey: 'nav.services', url: '/services', icon: IconNetwork },
  ],
  'sidebar.groups.storage': [
    {
      titleKey: 'sidebar.short.pvcs',
      url: '/persistentvolumeclaims',
      icon: IconFileDatabase,
    },
    {
      titleKey: 'sidebar.short.pvs',
      url: '/persistentvolumes',
      icon: IconDatabase,
    },
    {
      titleKey: 'nav.storageclasses',
      url: '/storageclasses',
      icon: IconFileDatabase,
    },
  ],
  'sidebar.groups.config': [
    { titleKey: 'nav.configMaps', url: '/configmaps', icon: IconMap },
    { titleKey: 'nav.secrets', url: '/secrets', icon: IconLock },
    {
      titleKey: 'nav.horizontalpodautoscalers',
      url: '/horizontalpodautoscalers',
      icon: IconArrowsHorizontal,
    },
  ],
  'sidebar.groups.security': [
    {
      titleKey: 'nav.serviceaccounts',
      url: '/serviceaccounts',
      icon: IconUser,
    },
    { titleKey: 'nav.roles', url: '/roles', icon: IconShield },
    { titleKey: 'nav.rolebindings', url: '/rolebindings', icon: IconUsers },
    {
      titleKey: 'nav.clusterroles',
      url: '/clusterroles',
      icon: IconShieldCheck,
    },
    {
      titleKey: 'nav.clusterrolebindings',
      url: '/clusterrolebindings',
      icon: IconKey,
    },
  ],
  'sidebar.groups.other': [
    {
      titleKey: 'nav.namespaces',
      url: '/namespaces',
      icon: IconBoxMultiple,
    },
    { titleKey: 'nav.nodes', url: '/nodes', icon: IconServer2 },
    { titleKey: 'nav.events', url: '/events', icon: IconBell },
    { titleKey: 'nav.crds', url: '/crds', icon: IconCode },
  ],
  [BUILTIN_CR_GROUP_KEY]: BUILTIN_CRD_NAMES.map((crdName) => ({
    titleKey: crdName,
    url: `/crds/${crdName}`,
    icon: IconCode,
  })),
}

const CURRENT_CONFIG_VERSION = 2

const sanitizeSidebarConfig = (
  config: SidebarConfig,
  options?: {
    canViewCustomCRDGroups?: boolean
  }
): SidebarConfig => {
  const canViewCustomCRDGroups = options?.canViewCustomCRDGroups ?? true
  const removedItemIds = new Set<string>()
  const removedGroupIds = new Set<string>()

  const groups = config.groups
    .filter((group) => {
      const shouldKeep = canViewCustomCRDGroups || !group.isCustom
      if (!shouldKeep) {
        removedGroupIds.add(group.id)
        group.items.forEach((item) => removedItemIds.add(item.id))
      }
      return shouldKeep
    })
    .map((group) => {
      const items = group.items
        .filter((item) => {
          const shouldHide = isSidebarPathHidden(item.url)
          if (shouldHide) {
            removedItemIds.add(item.id)
          }
          return !shouldHide
        })
        .map((item, index) => ({
          ...item,
          order: index,
        }))

      return {
        ...group,
        items,
      }
    })

  return {
    ...config,
    groups,
    pinnedItems: config.pinnedItems.filter(
      (itemId) => !removedItemIds.has(itemId)
    ),
    hiddenItems: config.hiddenItems.filter(
      (itemId) => !removedItemIds.has(itemId)
    ),
    groupOrder: config.groupOrder.filter(
      (groupId) => !removedGroupIds.has(groupId)
    ),
  }
}

const defaultConfigs = (): SidebarConfig => {
  const groups: SidebarGroup[] = []
  let groupOrder = 0

  Object.entries(defaultMenus).forEach(([groupKey, items]) => {
    const groupId = getSidebarGroupID(groupKey)
    const sidebarItems: SidebarItem[] = items.map((item, index) => ({
      id: `${groupId}-${item.url.replace(/[^a-zA-Z0-9]/g, '-')}`,
      titleKey: item.titleKey,
      url: item.url,
      icon: getIconName(item.icon),
      visible: true,
      pinned: false,
      order: index,
    }))

    groups.push({
      id: groupId,
      nameKey: groupKey,
      items: sidebarItems,
      visible: true,
      collapsed: false,
      order: groupOrder++,
    })
  })

  return {
    version: CURRENT_CONFIG_VERSION,
    groups,
    hiddenItems: [],
    pinnedItems: [],
    groupOrder: groups.map((g) => g.id),
    lastUpdated: Date.now(),
  }
}

export const SidebarConfigProvider: React.FC<SidebarConfigProviderProps> = ({
  children,
}) => {
  const [config, setConfig] = useState<SidebarConfig | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [hasUpdate, setHasUpdate] = useState(false)
  const hasLoadedConfigRef = useRef(false)
  const lastLoadedPreferenceRef = useRef<string | null>(null)
  const { user } = useAuth()
  const { data: builtinCRDs, isFetched: hasFetchedBuiltinCRDs } =
    useBuiltinSidebarCRDs({
      disable: !user,
    })
  const sidebarPreference = user?.sidebar_preference || ''
  const canCreateCustomCRDGroupPermission =
    user?.capabilities?.canCreateCustomCRDGroup ?? user?.isAdmin() ?? false
  const builtinCRDByName = useMemo(
    () => new Map((builtinCRDs ?? []).map((crd) => [crd.name, crd])),
    [builtinCRDs]
  )
  const availableBuiltinCRDs = useMemo(
    () =>
      new Set(
        (builtinCRDs ?? [])
          .map((crd) => crd.name)
          .filter((crdName): crdName is string => {
            if (typeof crdName !== 'string') {
              return false
            }
            return BUILTIN_CRD_NAME_SET.has(crdName)
          })
      ),
    [builtinCRDs]
  )
  const shouldShowSidebarItem = useCallback(
    (groupId: string, item: SidebarItem) => {
      if (groupId !== BUILTIN_CR_GROUP_ID) {
        return true
      }

      const crdName = getCRDNameFromSidebarURL(item.url)
      if (!crdName || !BUILTIN_CRD_NAME_SET.has(crdName)) {
        return false
      }

      if (!hasFetchedBuiltinCRDs) {
        return false
      }

      return availableBuiltinCRDs.has(crdName)
    },
    [availableBuiltinCRDs, hasFetchedBuiltinCRDs]
  )
  const resolveSidebarItemTitle = useCallback(
    (_groupId: string, item: SidebarItem) => {
      return item.titleKey
    },
    []
  )
  const getSidebarItemScope = useCallback(
    (groupId: string, item: SidebarItem) => {
      if (groupId !== BUILTIN_CR_GROUP_ID) {
        return undefined
      }

      const crdName = getCRDNameFromSidebarURL(item.url)
      if (!crdName) {
        return undefined
      }

      return builtinCRDByName.get(crdName)?.scope
    },
    [builtinCRDByName]
  )

  const loadConfig = useCallback(() => {
    const normalizedPreference = sidebarPreference.trim()
    const loadCacheKey = `${canCreateCustomCRDGroupPermission ? '1' : '0'}:${normalizedPreference}`

    if (lastLoadedPreferenceRef.current === loadCacheKey) {
      return
    }
    lastLoadedPreferenceRef.current = loadCacheKey

    if (normalizedPreference) {
      try {
        const userConfig = ensureBuiltinCRGroup(
          ensureApplicationGroup(JSON.parse(normalizedPreference))
        )
        const sanitizedConfig = sanitizeSidebarConfig(userConfig, {
          canViewCustomCRDGroups: canCreateCustomCRDGroupPermission,
        })
        setConfig(sanitizedConfig)

        const currentVersion = userConfig.version || 0
        setHasUpdate(currentVersion < CURRENT_CONFIG_VERSION)
        return
      } catch (error) {
        console.error('Failed to parse sidebar preference:', error)
      }
    }
    setHasUpdate(false)
    setConfig(
      sanitizeSidebarConfig(defaultConfigs(), {
        canViewCustomCRDGroups: canCreateCustomCRDGroupPermission,
      })
    )
  }, [sidebarPreference, canCreateCustomCRDGroupPermission])

  const saveConfig = useCallback(
    async (newConfig: SidebarConfig) => {
      const configWithBuiltinCRGroup = ensureBuiltinCRGroup(
        ensureApplicationGroup(newConfig)
      )
      const sanitizedConfig = sanitizeSidebarConfig(configWithBuiltinCRGroup, {
        canViewCustomCRDGroups: canCreateCustomCRDGroupPermission,
      })

      if (!user) {
        setConfig(sanitizedConfig)
        return
      }

      try {
        const configToSave = {
          ...sanitizedConfig,
          lastUpdated: Date.now(),
          version: CURRENT_CONFIG_VERSION,
        }

        const response = await fetch(
          withSubPath('/api/users/sidebar_preference'),
          {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
            },
            credentials: 'include',
            body: JSON.stringify({
              sidebar_preference: JSON.stringify(configToSave),
            }),
          }
        )

        if (response.ok) {
          setConfig(configToSave)
        } else {
          console.error('Failed to save sidebar config to server')
        }
      } catch (error) {
        console.error('Failed to save sidebar config to server:', error)
      }
    },
    [user, canCreateCustomCRDGroupPermission]
  )

  const updateConfig = useCallback(
    (updates: Partial<SidebarConfig>) => {
      if (!config) return
      const newConfig = { ...config, ...updates }
      saveConfig(newConfig)
    },
    [config, saveConfig]
  )

  const toggleItemVisibility = useCallback(
    (itemId: string) => {
      if (!config) return

      const hiddenItems = new Set(config.hiddenItems)
      if (hiddenItems.has(itemId)) {
        hiddenItems.delete(itemId)
      } else {
        hiddenItems.add(itemId)
      }

      updateConfig({ hiddenItems: Array.from(hiddenItems) })
    },
    [config, updateConfig]
  )

  const toggleItemPin = useCallback(
    (itemId: string) => {
      if (!config) return

      const pinnedItems = new Set(config.pinnedItems)
      if (pinnedItems.has(itemId)) {
        pinnedItems.delete(itemId)
      } else {
        pinnedItems.add(itemId)
      }

      updateConfig({ pinnedItems: Array.from(pinnedItems) })
    },
    [config, updateConfig]
  )

  const toggleGroupVisibility = useCallback(
    (groupId: string) => {
      if (!config) return

      const groups = config.groups.map((group) =>
        group.id === groupId ? { ...group, visible: !group.visible } : group
      )

      updateConfig({ groups })
    },
    [config, updateConfig]
  )

  const toggleGroupCollapse = useCallback(
    (groupId: string) => {
      if (!config) return

      const groups = config.groups.map((group) =>
        group.id === groupId ? { ...group, collapsed: !group.collapsed } : group
      )

      updateConfig({ groups })
    },
    [config, updateConfig]
  )

  const moveGroup = useCallback(
    (groupId: string, direction: 'up' | 'down') => {
      if (!config) return

      const sortedGroups = [...config.groups].sort((a, b) => a.order - b.order)
      const currentIndex = sortedGroups.findIndex(
        (group) => group.id === groupId
      )
      if (currentIndex === -1) return

      const targetIndex =
        direction === 'up' ? currentIndex - 1 : currentIndex + 1

      if (targetIndex < 0 || targetIndex >= sortedGroups.length) {
        return
      }

      const reordered = [...sortedGroups]
      const [movedGroup] = reordered.splice(currentIndex, 1)
      reordered.splice(targetIndex, 0, movedGroup)

      const groups = reordered.map((group, index) => ({
        ...group,
        order: index,
      }))
      const groupOrder = groups.map((group) => group.id)

      updateConfig({ groups, groupOrder })
    },
    [config, updateConfig]
  )

  const createCustomGroup = useCallback(
    (groupName: string) => {
      if (!config) return
      if (!canCreateCustomCRDGroupPermission) return

      const groupId = `custom-${groupName.toLowerCase().replace(/\s+/g, '-')}`

      // Check if group already exists
      if (config.groups.find((g) => g.id === groupId)) {
        return
      }

      const newGroup: SidebarGroup = {
        id: groupId,
        nameKey: groupName,
        items: [],
        visible: true,
        collapsed: false,
        order: config.groups.length,
        isCustom: true,
      }

      const groups = [...config.groups, newGroup]
      updateConfig({ groups, groupOrder: [...config.groupOrder, groupId] })
    },
    [canCreateCustomCRDGroupPermission, config, updateConfig]
  )

  const addCRDToGroup = useCallback(
    (groupId: string, crdName: string, kind: string) => {
      if (!config) return

      const groups = config.groups.map((group) => {
        if (group.id === groupId) {
          const itemId = `${groupId}-${crdName.replace(/[^a-zA-Z0-9]/g, '-')}`

          // Check if CRD already exists in this group
          if (group.items.find((item) => item.id === itemId)) {
            return group
          }

          const newItem: SidebarItem = {
            id: itemId,
            titleKey: kind,
            url: `/crds/${crdName}`,
            icon: 'IconCode',
            visible: true,
            pinned: false,
            order: group.items.length,
          }

          return {
            ...group,
            items: [...group.items, newItem],
          }
        }
        return group
      })

      updateConfig({ groups })
    },
    [config, updateConfig]
  )

  const removeCRDToGroup = useCallback(
    (groupId: string, itemID: string) => {
      if (!config) return
      const groups = config.groups.map((group) => {
        if (group.id === groupId) {
          const newItems = group.items.filter((item) => item.id !== itemID)
          return {
            ...group,
            items: newItems,
          }
        }
        return group
      })

      const pinnedItems = config.pinnedItems.filter((item) => item !== itemID)
      const hiddenItems = config.hiddenItems.filter((item) => item !== itemID)

      updateConfig({ groups, pinnedItems, hiddenItems })
    },
    [config, updateConfig]
  )

  const removeCustomGroup = useCallback(
    (groupId: string) => {
      if (!config) return

      // Only allow removing custom groups
      const group = config.groups.find((g) => g.id === groupId)
      if (!group?.isCustom) return

      const groups = config.groups.filter((g) => g.id !== groupId)
      const groupOrder = config.groupOrder.filter((id) => id !== groupId)

      // Remove any pinned items from this group
      const groupItemIds = group.items.map((item) => item.id)
      const pinnedItems = config.pinnedItems.filter(
        (itemId) => !groupItemIds.includes(itemId)
      )
      const hiddenItems = config.hiddenItems.filter(
        (itemId) => !groupItemIds.includes(itemId)
      )

      updateConfig({ groups, groupOrder, pinnedItems, hiddenItems })
    },
    [config, updateConfig]
  )

  const resetConfig = useCallback(() => {
    const newConfig = defaultConfigs()
    saveConfig(newConfig)
    setHasUpdate(false)
  }, [saveConfig])

  const getIconComponent = useCallback((iconName: string) => {
    return iconMap[iconName as keyof typeof iconMap] || IconBox
  }, [])

  useEffect(() => {
    const loadData = () => {
      if (!hasLoadedConfigRef.current) {
        setIsLoading(true)
      }
      loadConfig()
      hasLoadedConfigRef.current = true
      setIsLoading(false)
    }
    loadData()
  }, [loadConfig])

  const value: SidebarConfigContextType = {
    config,
    isLoading,
    hasUpdate,
    canCreateCustomCRDGroup: canCreateCustomCRDGroupPermission,
    shouldShowSidebarItem,
    resolveSidebarItemTitle,
    getSidebarItemScope,
    updateConfig,
    toggleItemVisibility,
    toggleGroupVisibility,
    toggleItemPin,
    toggleGroupCollapse,
    resetConfig,
    getIconComponent,
    createCustomGroup,
    addCRDToGroup,
    removeCRDToGroup,
    removeCustomGroup,
    moveGroup,
  }

  return (
    <SidebarConfigContext.Provider value={value}>
      {children}
    </SidebarConfigContext.Provider>
  )
}
