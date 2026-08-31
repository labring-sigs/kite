import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from 'react-router-dom'

import './index.css'
import './i18n'

import { AppearanceProvider } from './components/appearance-provider'
import { AuthProvider } from './contexts/auth-context'
import { SidebarConfigProvider } from './contexts/sidebar-config-context'
import { QueryProvider } from './lib/query-provider'
import { router } from './routes'

// NOTE: monaco-editor is intentionally NOT imported here. It is bootstrapped
// lazily by lib/monaco-loader.ts the first time an editor mounts, keeping the
// entry chunk free of the editor, its worker and the react bindings.

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryProvider>
      <AppearanceProvider
        defaultTheme="system"
        defaultColorTheme="default"
        defaultFont="system"
      >
        <AuthProvider>
          <SidebarConfigProvider>
            <RouterProvider router={router} />
          </SidebarConfigProvider>
        </AuthProvider>
      </AppearanceProvider>
    </QueryProvider>
  </StrictMode>
)
