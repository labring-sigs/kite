import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { useAuth } from '@/contexts/auth-context'
import {
  ColumnDef,
  ColumnFiltersState,
  getCoreRowModel,
  getFacetedRowModel,
  getFacetedUniqueValues,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  PaginationState,
  RowSelectionState,
  SortingState,
  useReactTable,
} from '@tanstack/react-table'
import {
  Box,
  Database,
  Plus,
  RefreshCw,
  Search,
  Settings2,
  Trash2,
  XCircle,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ResourceType } from '@/types/api'
import { deleteResource, useResources, useResourcesWatch } from '@/lib/api'
import { useCluster } from '@/hooks/use-cluster'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import { ConnectionIndicator } from './connection-indicator'
import { ErrorMessage } from './error-message'
import { ResourceTableView } from './resource-table-view'
import { NamespaceSelector } from './selector/namespace-selector'

export interface ResourceTableProps<T> {
  resourceName: string
  resourceType?: ResourceType // Optional, used for fetching resources
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  columns: ColumnDef<T, any>[]
  clusterScope?: boolean // If true, don't show namespace selector
  searchQueryFilter?: (item: T, query: string) => boolean // Custom filter function
  showCreateButton?: boolean // If true, show create button
  onCreateClick?: () => void // Callback for create button click
  extraToolbars?: React.ReactNode[] // Additional toolbar components
  defaultHiddenColumns?: string[] // Columns to hide by default
}

export function ResourceTable<T>({
  resourceName,
  resourceType,
  columns,
  clusterScope = false,
  searchQueryFilter,
  showCreateButton = false,
  onCreateClick,
  extraToolbars = [],
  defaultHiddenColumns = [],
}: ResourceTableProps<T>) {
  const { t } = useTranslation()
  const { user, isLoading: authLoading } = useAuth()
  const {
    currentCluster,
    currentClusterInfo,
    isLoading: clustersLoading,
  } = useCluster()
  const isAdminUser = user?.isAdmin() ?? false
  const defaultNamespace = isAdminUser ? '_all' : 'default'
  const clusterDefaultNamespace = currentClusterInfo?.namespace || undefined
  const fixedNamespace =
    !clusterScope && currentClusterInfo?.namespaceScoped
      ? currentClusterInfo.namespace
      : undefined
  const namespaceSelectionReady =
    clusterScope ||
    Boolean(fixedNamespace) ||
    Boolean(
      currentCluster && currentClusterInfo && !authLoading && !clustersLoading
    )
  const selectedNamespaceStorageKey = currentCluster
    ? `${currentCluster}selectedNamespace`
    : undefined
  const selectedNamespaceSourceKey = selectedNamespaceStorageKey
    ? `${selectedNamespaceStorageKey}:source`
    : undefined
  const [sorting, setSorting] = useState<SortingState>([])
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>(() => {
    const currentCluster = localStorage.getItem('current-cluster')
    const storageKey = `${currentCluster}-${resourceName}-columnFilters`
    const savedFilters = sessionStorage.getItem(storageKey)
    return savedFilters ? JSON.parse(savedFilters) : []
  })
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [isDeleting, setIsDeleting] = useState(false)
  const [searchQuery, setSearchQuery] = useState<string>(() => {
    const currentCluster = localStorage.getItem('current-cluster')
    const storageKey = `${currentCluster}-${resourceName}-searchQuery`
    return sessionStorage.getItem(storageKey) || ''
  })

  const [columnVisibility, setColumnVisibility] = useState<
    Record<string, boolean>
  >(() => {
    const currentCluster = localStorage.getItem('current-cluster')
    const storageKey = `${currentCluster}-${resourceName}-columnVisibility`
    const savedVisibility = localStorage.getItem(storageKey)
    if (savedVisibility) {
      return JSON.parse(savedVisibility)
    }
    // Set default hidden columns if no saved state
    const initialVisibility: Record<string, boolean> = {}
    defaultHiddenColumns.forEach((colId) => {
      initialVisibility[colId] = false
    })
    return initialVisibility
  })

  const [pagination, setPagination] = useState<PaginationState>(() => {
    const currentCluster = localStorage.getItem('current-cluster')
    const storageKey = `${currentCluster}-${resourceName}-pageSize`
    const savedPageSize = sessionStorage.getItem(storageKey)
    return {
      pageIndex: 0,
      pageSize: savedPageSize ? Number(savedPageSize) : 20,
    }
  })
  // Default auto-refresh is 10s so it aligns with the backend summary-list
  // cache TTL: a 5s poll mostly re-ran expensive list loads, while a 10s
  // poll is served straight from the warm cache.
  const [refreshInterval, setRefreshInterval] = useState(10000)

  const [selectedNamespace, setSelectedNamespace] = useState<
    string | undefined
  >(() => {
    // Try to get the stored namespace from localStorage
    const currentClusterName = localStorage.getItem('current-cluster')
    const storedNamespace = currentClusterName
      ? localStorage.getItem(`${currentClusterName}selectedNamespace`)
      : null
    return clusterScope
      ? undefined // No namespace for cluster scope
      : storedNamespace || defaultNamespace
  })
  const preferredNamespace = clusterDefaultNamespace || defaultNamespace
  const storedNamespace =
    selectedNamespaceStorageKey && namespaceSelectionReady
      ? localStorage.getItem(selectedNamespaceStorageKey)
      : null
  const storedNamespaceSource =
    selectedNamespaceSourceKey && namespaceSelectionReady
      ? localStorage.getItem(selectedNamespaceSourceKey)
      : null
  const shouldRepairLegacyDefaultFallback =
    storedNamespace === 'default' &&
    storedNamespaceSource !== 'manual' &&
    isAdminUser &&
    Boolean(clusterDefaultNamespace) &&
    clusterDefaultNamespace !== 'default'
  const storedNamespaceIsAutomatic =
    storedNamespaceSource === 'fallback' ||
    storedNamespaceSource === 'fixed' ||
    shouldRepairLegacyDefaultFallback
  const targetNamespace =
    storedNamespace && !storedNamespaceIsAutomatic
      ? storedNamespace
      : preferredNamespace
  const effectiveSelectedNamespace =
    namespaceSelectionReady && !clusterScope && !fixedNamespace
      ? targetNamespace
      : selectedNamespace
  const {
    data: availableNamespaces,
    isPlaceholderData: namespacesArePlaceholder,
  } = useResources('namespaces', undefined, {
    disable: clusterScope || !!fixedNamespace || !namespaceSelectionReady,
  })
  const namespaceNames = useMemo(
    () =>
      (availableNamespaces || [])
        .map((namespace) => namespace.metadata?.name)
        .filter((name): name is string => Boolean(name)),
    [availableNamespaces]
  )
  const namespaceValidationRequired =
    namespaceSelectionReady &&
    !clusterScope &&
    !fixedNamespace &&
    Boolean(effectiveSelectedNamespace) &&
    effectiveSelectedNamespace !== '_all'
  const namespaceValidationPending =
    namespaceValidationRequired &&
    (namespacesArePlaceholder || !availableNamespaces)
  const selectedNamespaceExists =
    effectiveSelectedNamespace &&
    namespaceNames.includes(effectiveSelectedNamespace)
  const missingNamespaceFallback =
    clusterDefaultNamespace && namespaceNames.includes(clusterDefaultNamespace)
      ? clusterDefaultNamespace
      : defaultNamespace === '_all' || namespaceNames.includes(defaultNamespace)
        ? defaultNamespace
        : namespaceNames[0]
  const validatedSelectedNamespace =
    namespaceValidationRequired &&
    !namespaceValidationPending &&
    !selectedNamespaceExists
      ? missingNamespaceFallback
      : effectiveSelectedNamespace
  const requestNamespace = clusterScope
    ? undefined
    : fixedNamespace || validatedSelectedNamespace
  const [useSSE, setUseSSE] = useState(false)
  const resolvedResourceType = (resourceType ??
    (resourceName.toLowerCase() as ResourceType)) as ResourceType
  const {
    isLoading: queryLoading,
    data: queryData,
    isError: queryIsError,
    error: queryError,
    refetch: queryRefetch,
  } = useResources(resolvedResourceType, requestNamespace, {
    refreshInterval: useSSE ? 0 : refreshInterval, // disable polling when SSE
    reduce: true, // Fetch reduced data for performance
    disable:
      useSSE ||
      !namespaceSelectionReady ||
      namespaceValidationPending ||
      (!clusterScope && !fixedNamespace && !validatedSelectedNamespace),
  })

  // SSE state (when enabled)
  // SSE watch hook
  const {
    data: watchData,
    isLoading: watchLoading,
    error: watchError,
    isConnected,
    refetch: reconnectSSE,
  } = useResourcesWatch(resolvedResourceType, requestNamespace, {
    reduce: true,
    enabled:
      useSSE &&
      namespaceSelectionReady &&
      !namespaceValidationPending &&
      Boolean(clusterScope || fixedNamespace || validatedSelectedNamespace),
  })

  // (moved below after error is defined)

  // Update sessionStorage when search query changes
  useEffect(() => {
    const currentCluster = localStorage.getItem('current-cluster')
    const storageKey = `${currentCluster}-${resourceName}-searchQuery`
    if (searchQuery) {
      sessionStorage.setItem(storageKey, searchQuery)
    } else {
      sessionStorage.removeItem(storageKey)
    }
  }, [searchQuery, resourceName])

  // Update sessionStorage when column visibility changes
  useEffect(() => {
    const currentCluster = localStorage.getItem('current-cluster')
    const storageKey = `${currentCluster}-${resourceName}-columnVisibility`
    localStorage.setItem(storageKey, JSON.stringify(columnVisibility))
  }, [columnVisibility, resourceName])

  // Update sessionStorage when page size changes
  useEffect(() => {
    const currentCluster = localStorage.getItem('current-cluster')
    const storageKey = `${currentCluster}-${resourceName}-pageSize`
    sessionStorage.setItem(storageKey, pagination.pageSize.toString())
  }, [pagination.pageSize, resourceName])

  // Update sessionStorage when column filters changes
  useEffect(() => {
    const currentCluster = localStorage.getItem('current-cluster')
    const storageKey = `${currentCluster}-${resourceName}-columnFilters`
    if (columnFilters.length > 0) {
      sessionStorage.setItem(storageKey, JSON.stringify(columnFilters))
    } else {
      sessionStorage.removeItem(storageKey)
    }
  }, [columnFilters, resourceName])

  // Reset pagination when filters change
  useEffect(() => {
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }, [columnFilters, searchQuery])

  useEffect(() => {
    if (
      clusterScope ||
      !currentCluster ||
      !namespaceSelectionReady ||
      !selectedNamespaceStorageKey ||
      !selectedNamespaceSourceKey
    ) {
      return
    }

    if (fixedNamespace) {
      localStorage.setItem(selectedNamespaceStorageKey, fixedNamespace)
      localStorage.setItem(selectedNamespaceSourceKey, 'fixed')
      if (selectedNamespace !== fixedNamespace) {
        setSelectedNamespace(fixedNamespace)
      }
      return
    }

    if (storedNamespaceIsAutomatic && storedNamespace !== targetNamespace) {
      localStorage.setItem(selectedNamespaceStorageKey, targetNamespace)
      localStorage.setItem(selectedNamespaceSourceKey, 'fallback')
    }

    if (selectedNamespace !== targetNamespace) {
      setSelectedNamespace(targetNamespace)
    }
  }, [
    clusterScope,
    clusterDefaultNamespace,
    currentCluster,
    defaultNamespace,
    fixedNamespace,
    namespaceSelectionReady,
    selectedNamespace,
    selectedNamespaceSourceKey,
    selectedNamespaceStorageKey,
    storedNamespace,
    storedNamespaceIsAutomatic,
    targetNamespace,
  ])

  useEffect(() => {
    if (
      clusterScope ||
      fixedNamespace ||
      !currentCluster ||
      !namespaceSelectionReady ||
      !selectedNamespaceStorageKey ||
      !selectedNamespaceSourceKey ||
      !effectiveSelectedNamespace ||
      effectiveSelectedNamespace === '_all' ||
      namespaceValidationPending ||
      !availableNamespaces
    ) {
      return
    }

    if (selectedNamespaceExists) {
      return
    }

    if (!missingNamespaceFallback) {
      return
    }

    localStorage.setItem(selectedNamespaceStorageKey, missingNamespaceFallback)
    localStorage.setItem(selectedNamespaceSourceKey, 'fallback')
    setSelectedNamespace(missingNamespaceFallback)
  }, [
    availableNamespaces,
    clusterScope,
    currentCluster,
    effectiveSelectedNamespace,
    fixedNamespace,
    missingNamespaceFallback,
    namespaceSelectionReady,
    namespaceValidationPending,
    selectedNamespaceExists,
    selectedNamespaceSourceKey,
    selectedNamespaceStorageKey,
  ])

  // Handle namespace change
  const handleNamespaceChange = useCallback(
    (value: string) => {
      if (fixedNamespace) return
      if (setSelectedNamespace && selectedNamespaceStorageKey) {
        localStorage.setItem(selectedNamespaceStorageKey, value)
        localStorage.setItem(`${selectedNamespaceStorageKey}:source`, 'manual')
        setSelectedNamespace(value)
        // Reset pagination and search when changing namespace
        setPagination({ pageIndex: 0, pageSize: pagination.pageSize })
        setSearchQuery('')
      }
    },
    [
      fixedNamespace,
      setSelectedNamespace,
      selectedNamespaceStorageKey,
      pagination.pageSize,
    ]
  )

  // Add namespace column when showing all namespaces
  const enhancedColumns = useMemo(() => {
    const selectColumn: ColumnDef<T> = {
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={
            table.getIsAllPageRowsSelected() ||
            (table.getIsSomePageRowsSelected() && 'indeterminate')
          }
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label={t('resourceTable.selectAllAria')}
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label={t('resourceTable.selectRowAria')}
        />
      ),
      enableSorting: false,
      enableHiding: false,
    }

    const baseColumns = [selectColumn, ...columns]

    // Only add namespace column if not cluster scope, showing all namespaces,
    // and there isn't already a namespace column in the provided columns
    if (!clusterScope && selectedNamespace === '_all') {
      // Check if namespace column already exists in the provided columns
      const hasNamespaceColumn = columns.some((col) => {
        // Check if the column accesses namespace data
        if ('accessorKey' in col && col.accessorKey === 'metadata.namespace') {
          return true
        }
        if ('accessorFn' in col && col.id === 'namespace') {
          return true
        }
        return false
      })

      // Only add namespace column if it doesn't already exist
      if (!hasNamespaceColumn) {
        const namespaceColumn = {
          id: 'namespace',
          header: t('resourceTable.namespace'),
          accessorFn: (row: T) => {
            // Try to get namespace from metadata.namespace
            const metadata = (row as { metadata?: { namespace?: string } })
              ?.metadata
            return metadata?.namespace || '-'
          },
          cell: ({ getValue }: { getValue: () => string }) => (
            <Badge variant="outline" className="ml-2 ">
              {getValue()}
            </Badge>
          ),
        }

        // Insert namespace column after select and first column (typically name)
        const columnsWithNamespace = [...baseColumns]
        columnsWithNamespace.splice(2, 0, namespaceColumn)
        return columnsWithNamespace
      }
    }
    return baseColumns
  }, [columns, clusterScope, selectedNamespace, t])

  const data = useMemo(() => {
    if (useSSE) return watchData
    return queryData
  }, [useSSE, watchData, queryData])
  const isLoading = useSSE ? watchLoading : queryLoading
  const isError = useSSE ? Boolean(watchError) : queryIsError
  const error = useSSE
    ? (watchError as Error | null)
    : (queryError as unknown as Error | null)
  const refetch = useSSE ? reconnectSSE : queryRefetch

  const memoizedData = useMemo(() => (data || []) as T[], [data])

  useEffect(() => {
    if (!useSSE && error) {
      setRefreshInterval(0)
    }
  }, [useSSE, error])

  // Create table instance using TanStack Table
  const table = useReactTable<T>({
    data: memoizedData,
    columns: enhancedColumns,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getFacetedRowModel: getFacetedRowModel(),
    getFacetedUniqueValues: getFacetedUniqueValues(),
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    onRowSelectionChange: setRowSelection,
    onColumnVisibilityChange: setColumnVisibility,
    getRowId: (row) => {
      const metadata = (
        row as {
          metadata?: { name?: string; namespace?: string; uid?: string }
        }
      )?.metadata
      if (!metadata?.name) {
        return `row-${Math.random()}`
      }
      return (
        metadata.uid ||
        (metadata.namespace
          ? `${metadata.namespace}/${metadata.name}`
          : metadata.name)
      )
    },
    state: {
      sorting,
      columnFilters,
      globalFilter: searchQuery,
      pagination,
      rowSelection,
      columnVisibility,
    },
    onPaginationChange: setPagination,
    // Let TanStack Table handle pagination automatically based on filtered data
    manualPagination: false,
    // Improve filtering performance and consistency
    globalFilterFn: (row, _columnId, value) => {
      if (searchQueryFilter) {
        return searchQueryFilter(row.original as T, String(value).toLowerCase())
      }
      const searchValue = String(value).toLowerCase()

      // Search across all visible columns
      return row.getVisibleCells().some((cell) => {
        const cellValue = String(cell.getValue() || '').toLowerCase()
        return cellValue.includes(searchValue)
      })
    },
    // Add this to prevent unnecessary pagination resets
    autoResetPageIndex: false,
    enableRowSelection: true,
  })

  // Handle batch delete - must be after table is defined
  const handleBatchDelete = useCallback(async () => {
    setIsDeleting(true)
    const selectedRows = table
      .getSelectedRowModel()
      .rows.map((row) => row.original)

    const deletePromises = selectedRows.map((row) => {
      const metadata = (
        row as { metadata?: { name?: string; namespace?: string } }
      )?.metadata
      const name = metadata?.name
      const namespace = clusterScope ? undefined : metadata?.namespace

      if (!name) {
        return Promise.resolve()
      }

      return deleteResource(resolvedResourceType, name, namespace)
        .then(() => {
          toast.success(t('resourceTable.deleteSuccess', { name }))
        })
        .catch((error) => {
          console.error(`Failed to delete ${name}:`, error)
          toast.error(
            t('resourceTable.deleteFailed', { name, error: error.message })
          )
          throw error
        })
    })

    try {
      await Promise.allSettled(deletePromises)
      // Reset selection and close dialog
      setRowSelection({})
      setDeleteDialogOpen(false)
      // Refetch data
      if (!useSSE) {
        refetch()
      }
    } finally {
      setIsDeleting(false)
    }
  }, [table, clusterScope, resolvedResourceType, t, useSSE, refetch])
  // Calculate total and filtered row counts
  const totalRowCount = useMemo(
    () => (data as T[] | undefined)?.length || 0,
    [data]
  )
  const filteredRowCount = useMemo(() => {
    if (!data || (data as T[]).length === 0) return 0
    // Force re-computation when filters change
    void searchQuery // Ensure dependency is used
    void columnFilters // Ensure dependency is used
    return table.getFilteredRowModel().rows.length
  }, [table, data, searchQuery, columnFilters])

  // Check if there are active filters
  const hasActiveFilters = useMemo(() => {
    return Boolean(searchQuery) || columnFilters.length > 0
  }, [searchQuery, columnFilters])

  // Render empty state based on condition
  const renderEmptyState = () => {
    // Only show loading state if there's no existing data
    if (isLoading && (!data || (data as T[]).length === 0)) {
      return (
        <div className="h-72 flex flex-col items-center justify-center">
          <div className="mb-4 bg-muted/30 p-6 rounded-full">
            <Database className="h-12 w-12 text-muted-foreground animate-pulse" />
          </div>
          <h3 className="text-lg font-medium mb-1">
            {t('resourceTable.loadingResource', {
              resource: resourceName.toLowerCase(),
            })}
          </h3>
          <p className="text-muted-foreground">
            {t('resourceTable.retrievingData')}
            {!clusterScope && selectedNamespace
              ? ` ${t(
                  selectedNamespace === '_all'
                    ? 'resourceTable.fromAllNamespaces'
                    : 'resourceTable.fromNamespace',
                  { namespace: selectedNamespace }
                )}`
              : ''}
          </p>
        </div>
      )
    }

    if (isError) {
      return (
        <ErrorMessage
          resourceName={resourceName}
          error={error}
          refetch={refetch}
        />
      )
    }

    if (data && (data as T[]).length === 0) {
      return (
        <div className="h-72 flex flex-col items-center justify-center">
          <div className="mb-4 bg-muted/30 p-6 rounded-full">
            <Box className="h-12 w-12 text-muted-foreground" />
          </div>
          <h3 className="text-lg font-medium mb-1">
            {t('resourceTable.noResourceFound', {
              resource: resourceName.toLowerCase(),
            })}
          </h3>
          <p className="text-muted-foreground">
            {searchQuery
              ? t('resourceTable.noResultsMatchQuery', { query: searchQuery })
              : clusterScope
                ? t('resourceTable.noResourceInCluster', {
                    resource: resourceName.toLowerCase(),
                  })
                : t('resourceTable.noResourceInNamespace', {
                    resource: resourceName.toLowerCase(),
                    namespace: selectedNamespace,
                  })}
          </p>
          {searchQuery && (
            <Button
              variant="outline"
              className="mt-4"
              onClick={() => setSearchQuery('')}
            >
              {t('resourceTable.clearSearch')}
            </Button>
          )}
        </div>
      )
    }

    return null
  }

  const emptyState = renderEmptyState()

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold capitalize">{resourceName}</h1>
          {!clusterScope && selectedNamespace && (
            <div className="text-muted-foreground flex items-center mt-1">
              <span>{t('resourceTable.namespaceLabel')}</span>
              <Badge variant="outline" className="ml-2 ">
                {selectedNamespace === '_all'
                  ? t('rbac.allNamespaces')
                  : selectedNamespace}
              </Badge>
            </div>
          )}
        </div>

        <div className="flex flex-col sm:flex-row items-start sm:items-center gap-4">
          <div className="flex items-center gap-2 flex-wrap">
            {extraToolbars?.map((toolbar, index) => (
              <React.Fragment key={index}>{toolbar}</React.Fragment>
            ))}
            {/* Watch/Live mode toggle switch */}
            {resolvedResourceType === 'pods' && (
              <div className="flex items-center gap-2">
                <Label className="text-sm">
                  {useSSE ? (
                    <ConnectionIndicator isConnected={isConnected}>
                      {t('resourceTable.watch')}
                    </ConnectionIndicator>
                  ) : (
                    t('resourceTable.watch')
                  )}
                </Label>
                <Switch
                  checked={useSSE}
                  onCheckedChange={(checked) => {
                    setUseSSE(checked)
                    if (checked) {
                      setRefreshInterval(0)
                    } else if (refreshInterval === 0) {
                      setRefreshInterval(5000) // Default to 5s when disabling watch mode
                    }
                  }}
                />
              </div>
            )}
            {/* Refresh interval selector */}
            <Select
              value={refreshInterval.toString()}
              onValueChange={(value) => {
                setRefreshInterval(Number(value))
                if (Number(value) > 0) {
                  setUseSSE(false)
                }
              }}
              disabled={useSSE}
            >
              <SelectTrigger className="max-w-[140px]">
                <div className="flex items-center gap-2">
                  <RefreshCw className="h-4 w-4" />
                  <SelectValue />
                </div>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="0">{t('resourceTable.off')}</SelectItem>
                <SelectItem value="1000">1s</SelectItem>
                <SelectItem value="5000">5s</SelectItem>
                <SelectItem value="10000">10s</SelectItem>
                <SelectItem value="30000">30s</SelectItem>
              </SelectContent>
            </Select>
            {!clusterScope && !fixedNamespace && (
              <NamespaceSelector
                selectedNamespace={selectedNamespace}
                handleNamespaceChange={handleNamespaceChange}
                showAll={true}
              />
            )}
            {/* Column Filters */}
            {table
              .getAllColumns()
              .filter((column) => {
                const columnDef = column.columnDef as ColumnDef<T> & {
                  enableColumnFilter?: boolean
                }
                return columnDef.enableColumnFilter && column.getCanFilter()
              })
              .map((column) => {
                const columnDef = column.columnDef as ColumnDef<T> & {
                  enableColumnFilter?: boolean
                }
                const uniqueValues = column.getFacetedUniqueValues()
                const filterValue = column.getFilterValue() as string

                return (
                  <Select
                    key={column.id}
                    value={filterValue || ''}
                    onValueChange={(value) =>
                      column.setFilterValue(value === 'all' ? '' : value)
                    }
                  >
                    <SelectTrigger className="min-w-32">
                      <SelectValue
                        placeholder={t('resourceTable.filterByColumn', {
                          column:
                            typeof columnDef.header === 'string'
                              ? columnDef.header
                              : t('resourceTable.column'),
                        })}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">
                        {t('resourceTable.allColumnValues', {
                          column:
                            typeof columnDef.header === 'string'
                              ? columnDef.header
                              : t('resourceTable.values'),
                        })}
                      </SelectItem>
                      {Array.from(uniqueValues.keys())
                        .sort()
                        .map((value) =>
                          value ? (
                            <SelectItem
                              key={String(value)}
                              value={String(value)}
                            >
                              {String(value)} ({uniqueValues.get(value)})
                            </SelectItem>
                          ) : null
                        )}
                    </SelectContent>
                  </Select>
                )
              })}
          </div>

          {/* Search bar */}
          <div className="flex items-center gap-2 w-full sm:w-auto">
            <div className="relative w-full sm:w-auto">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder={t('resourceTable.searchResource', {
                  resource: resourceName.toLowerCase(),
                })}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-9 pr-4 w-full sm:w-[100px] md:w-[200px]"
              />
            </div>
            {searchQuery && (
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setSearchQuery('')}
                className="h-9 w-9"
              >
                <XCircle className="h-4 w-4" />
              </Button>
            )}
          </div>
          {/* Batch delete button */}
          {table.getSelectedRowModel().rows.length > 0 && (
            <Button
              variant="destructive"
              onClick={() => setDeleteDialogOpen(true)}
              className="gap-2"
            >
              <Trash2 className="h-4 w-4" />
              {t('resourceTable.deleteSelected', {
                count: table.getSelectedRowModel().rows.length,
              })}
            </Button>
          )}
          {showCreateButton && onCreateClick && (
            <Button onClick={onCreateClick} className="gap-1">
              <Plus className="h-2 w-2" />
              {t('resourceTable.new')}
            </Button>
          )}

          {/* Toggle columns Dropdown */}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" size="icon" className="h-9 w-9">
                <Settings2 className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuLabel>
                {t('resourceTable.toggleColumns')}
              </DropdownMenuLabel>
              <DropdownMenuSeparator />
              {table
                .getAllLeafColumns()
                .filter((column) => column.getCanHide())
                .map((column) => {
                  const header = column.columnDef.header
                  const headerText =
                    typeof header === 'string' ? header : column.id
                  return (
                    <DropdownMenuCheckboxItem
                      key={column.id}
                      className="capitalize"
                      checked={column.getIsVisible()}
                      onCheckedChange={(value) =>
                        column.toggleVisibility(!!value)
                      }
                    >
                      {headerText}
                    </DropdownMenuCheckboxItem>
                  )
                })}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <ResourceTableView
        table={table}
        columnCount={enhancedColumns.length}
        isLoading={isLoading}
        data={data as T[] | undefined}
        emptyState={emptyState}
        hasActiveFilters={hasActiveFilters}
        filteredRowCount={filteredRowCount}
        totalRowCount={totalRowCount}
        searchQuery={searchQuery}
        pagination={pagination}
        setPagination={setPagination}
      />

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('resourceTable.confirmDeletion')}</DialogTitle>
            <DialogDescription>
              {t('resourceTable.confirmDeletionMessage', {
                count: table.getSelectedRowModel().rows.length,
                resourceName: resourceName.toLowerCase(),
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteDialogOpen(false)}
              disabled={isDeleting}
            >
              {t('common.cancel')}
            </Button>
            <Button
              variant="destructive"
              onClick={handleBatchDelete}
              disabled={isDeleting}
            >
              {isDeleting ? t('resourceTable.deleting') : t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
