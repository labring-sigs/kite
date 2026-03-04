import { Namespace } from 'kubernetes-types/core/v1'

import { useResources } from '@/lib/api'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export function NamespaceSelector({
  selectedNamespace,
  handleNamespaceChange,
  showAll = false,
  disabled = false,
}: {
  selectedNamespace?: string
  handleNamespaceChange: (namespace: string) => void
  showAll?: boolean
  disabled?: boolean
}) {
  const { data, isLoading } = useResources('namespaces', undefined, {
    disable: disabled,
  })

  const sortedNamespaces = (data ? [...data] : []).sort((a, b) => {
    const nameA = a.metadata?.name?.toLowerCase() || ''
    const nameB = b.metadata?.name?.toLowerCase() || ''
    return nameA.localeCompare(nameB)
  })

  const fallbackNamespace =
    selectedNamespace && !(showAll && selectedNamespace === '_all')
      ? selectedNamespace
      : undefined

  const namespaces = sortedNamespaces.length
    ? sortedNamespaces
    : fallbackNamespace
      ? [{ metadata: { name: fallbackNamespace } }]
      : []

  return (
    <Select
      value={selectedNamespace}
      onValueChange={handleNamespaceChange}
      disabled={disabled}
    >
      <SelectTrigger className="max-w-48">
        <SelectValue placeholder="Select a namespace" />
      </SelectTrigger>
      <SelectContent>
        {!disabled && isLoading && (
          <SelectItem disabled value="_loading">
            Loading namespaces...
          </SelectItem>
        )}
        {showAll && !disabled && (
          <SelectItem key="all" value="_all">
            All Namespaces
          </SelectItem>
        )}
        {namespaces.map((ns: Namespace) => (
          <SelectItem key={ns.metadata!.name} value={ns.metadata!.name!}>
            {ns.metadata!.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
