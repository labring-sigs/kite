# Information Architecture

This document summarizes the main route and navigation structure for Kite.

## Top-Level Routes

| Route | Component | Purpose |
| --- | --- | --- |
| `/login` | `LoginPage` | Standalone page for auth/session failures and, in Sealos deployments opened outside the Sealos desktop, the "open from Sealos Desktop" entry notice. It is not an interactive login form. |
| `/` | `App` with `ProtectedRoute` | Authenticated application shell with sidebar, header, global search, and resource routes. |

All routes respect the optional `KITE_BASE` subpath through `getSubPath()` and React Router `basename`.

## Auth Gates

- `ProtectedRoute` wraps `/`, sending missing users to `/login?reason=unauthenticated`.
- Auth refresh and API auth failures use reason-coded `/login` redirects so the fault page can explain the likely operator-owned issue.
- First-run bootstrap no longer uses a setup wizard: users and clusters come from boot-time environment configuration (`KITE_USERNAME`/`KITE_PASSWORD` and kubeconfig import) or from the Sealos login flow, which upserts both automatically.

## Authenticated App Shell

The authenticated shell includes:

- `AppSidebar` for resource navigation and customization.
- `SiteHeader` for current page controls.
- `GlobalSearch` for cross-resource search.
- `ClusterProvider` and cluster-aware API client configuration.

When `?iframe=true` is present, the app renders the route outlet without the standard shell.

## Resource Routes

| Route | Purpose |
| --- | --- |
| `/` and `/dashboard` | Cluster overview. |
| `/settings` | AI Agent settings page reached from the AI chat panel configure button, which is visible only to Kite administrators. Administrators can edit provider, API key, and model endpoint settings, while non-admin users who reach the route directly see an administrator-managed notice. |
| `/crds/:crd` | Generic Custom Resource list page for a selected CRD. |
| `/crds/:resource/:namespace/:name` | Namespaced Custom Resource detail page. |
| `/crds/:resource/:name` | Cluster-scoped Custom Resource detail page. |
| `/:resource` | Generic built-in resource list page. |
| `/:resource/:namespace/:name` | Namespaced resource detail page. |
| `/:resource/:name` | Cluster-scoped resource detail page. |

## Standalone State Rules

- `/login` should remain standalone and should not include the authenticated dashboard sidebar.
- `/login` must use the Kite logo and operator-focused copy for configuration, database, or authentication-service problems.
- Full app navigation should appear only when the route actually allows the user to navigate authenticated resource workflows.
