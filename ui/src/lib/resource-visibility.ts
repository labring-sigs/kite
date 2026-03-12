export const HIDDEN_RESOURCE_TYPES = ['gateways', 'httproutes'] as const

const hiddenResourceTypeSet = new Set<string>(HIDDEN_RESOURCE_TYPES)

export const isResourceTypeHidden = (resourceType?: string | null): boolean => {
  if (!resourceType) {
    return false
  }
  return hiddenResourceTypeSet.has(resourceType.toLowerCase())
}

export const isSidebarPathHidden = (path?: string | null): boolean => {
  if (!path) {
    return false
  }

  const pathWithoutQuery = path.split('?')[0]
  const firstSegment = pathWithoutQuery.replace(/^\/+/, '').split('/')[0]
  return isResourceTypeHidden(firstSegment)
}
