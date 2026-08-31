# Architecture

Kite is a single Go service that serves a React application and exposes REST/WebSocket APIs for Kubernetes operations.

## Runtime Shape

- `main.go` starts a Gin HTTP server on `PORT` (default `8080`), initializes configuration, connects to the database, runs migrations, initializes RBAC and templates, then registers API and static routes.
- The frontend is a Vite React app under `ui/`. Production builds are embedded into the Go binary through `//go:embed static`.
- The backend uses GORM with SQLite, MySQL, or PostgreSQL. `DB_TYPE`, `DB_DSN`, and `DB_AUTO_CREATE` control connection behavior.
- Kubernetes access is managed through `pkg/cluster`, `pkg/kube`, and middleware that resolves the current cluster from authenticated requests.

## Request Flow

1. Browser requests load the embedded React app from `static/index.html`.
2. React Router defines a standalone `/login` route, then protected app routes under `/`.
3. `ProtectedRoute` checks frontend auth state; missing sessions redirect to `/login?reason=unauthenticated`.
4. Protected backend APIs run through `RequireAuth`, cluster middleware, and RBAC middleware before resource handlers.

## Auth And Session Model

- Auth routes live under `/api/auth`.
- Backend interactive auth APIs still include password login, OAuth start/callback, Sealos login, logout, refresh, and current-user lookup.
- The frontend `/login` route is not an interactive login surface. It is an operational fault page used when no usable access session can be established or an auth callback/session refresh fails.
- Session state uses an `auth_token` cookie. `RequireAuth` accepts bearer tokens, API key tokens, or the cookie depending on request type.

## Data Model

Core persistent models are initialized in `pkg/model/model.go`:

- `User`
- `Cluster`
- `OAuthProvider`
- `Role`
- `RoleAssignment`
- `ResourceHistory`
- `ResourceTemplate`
- `GeneralSetting`
- `PendingSession`
- `HelmRepository`
- `ScheduledTask`

Kite runs `AutoMigrate` on startup. When `DB_AUTO_CREATE=true`, the service attempts to create a missing MySQL/PostgreSQL target database before migrations, so the configured database user must have the required permission.

## Resource Handling

- Generic Kubernetes resource list/detail/apply routes are registered from `pkg/handlers/resources`.
- Custom Resources are handled through `pkg/handlers/resources/cr_handler.go` and frontend routes under `/crds/...`.
- Search, logs, web terminal, node terminal, optional kubectl terminal, image tags, templates, Prometheus metrics, and proxy routes are registered under `/api/v1`.
- `pkg/ai` exposes `/api/v1/ai/chat` plus continuation endpoints. New installs enable the AI Agent UI by default, but `LoadRuntimeConfig` still disables runtime chat when no API key is configured. Tools read resources, logs, cluster overview, Helm releases, and Prometheus data through the current user and cluster context. In namespace-scoped non-exempt Sealos workspaces, AI tools resolve omitted namespace and `_all` to the current workspace namespace, deny ordinary cluster-scoped resources such as Nodes/Namespaces, hide arbitrary Prometheus querying, and reduce cluster overview to namespace-local pod/service counts. Mutating tools create a pending session and execute only after the user continues the confirmed operation.
- AI Helm operations are structured tool calls, not arbitrary shell commands in the Kite pod. The tools use the Helm v4 SDK through `pkg/helmutil`, reuse the same chart-source resolution as the HTTP Helm release API, run install/upgrade previews before writes, and pass rendered resources through `pkg/helmguard` before a release mutation reaches the cluster.
- `pkg/helm` exposes authenticated read-only chart catalog APIs under `/api/v1/charts` so namespaced Sealos users can browse and install charts they are authorized to render. Repository create/delete remains admin-only under `/api/v1/admin/charts`, and stored repository credentials must never be exposed in responses. Chart sources can come from admin-managed Helm repositories or controlled offline OCI discovery under a configured registry repository prefix. Offline OCI chart installs can also enable `pkg/helmutil` image policy preparation, which injects offline image defaults, checks rendered workload images, and rejects releases whose images still point outside the configured offline registry. Admin offline application bundle APIs package selected OCI charts with their rendered workload image archives into `.kiteapp.tar.gz` files; import validates the bundle, pushes images first, and pushes the chart last so catalog entries are not exposed before their required images exist. The preferred import path starts an in-memory background job after the upload is saved; job status is pollable by ID, but it is not persisted across pod restarts.
- `pkg/handlers/resources/helmrelease_handler.go` registers `helmreleases` as the canonical resource route, with a legacy `helmrelease` alias for compatibility. Helm install, upgrade, rollback, uninstall, and auto-upgrade pass through rendered-manifest authorization in `pkg/helmguard` before Helm writes resources. Dry-run previews use a separate Helm action configuration from the actual write because Helm's dry-run client mutates the action configuration's Kubernetes client behavior.
- `pkg/scheduler` runs background scheduled tasks such as Helm release auto-upgrade. Tasks reload their creator and due/enabled state before execution.

## Sealos Compatibility Layer

- `/api/auth/login/sealos` remains available when `SEALOS_AUTH_ENABLED=true`.
- Sealos SDK auto-login is handled in the frontend auth context and stores the selected cluster in storage plus the `x-cluster-name` cookie/header.
- Sealos SSO creates or updates a managed cluster, a per-user Sealos role, and a role assignment on login. When the workspace namespace is listed in `KITE_NAMESPACE_SCOPE_EXEMPT_NAMESPACES`, Kite treats that workspace as global/admin: the Sealos role receives `*` namespaces on its managed cluster, and the user also receives the built-in `admin` role for admin-only app settings such as AI Agent configuration. When the same Sealos user logs into a non-exempt workspace, Kite removes that user's stale built-in `admin` assignment before RBAC sync so global/admin access does not carry into a personal namespace.
- Sealos kubeconfigs with a non-exempt current-context namespace are loaded as namespace-scoped clients. Kite disables the controller-runtime informer cache for those clients and uses direct API reads to avoid broad cache startup behavior in personal workspaces.
- The frontend does not reject Sealos auth just because it is running as the top-level window. This keeps standalone local development and tools such as `sealos-app-dev-bridge` on the same SDK auto-login path as iframe deployments.
- `/login` may show a Sealos SDK availability notice when the SDK session channel is unavailable, but the notice is diagnostic-only and does not block the auto-login attempt.

## Static Assets And Base Path

`KITE_BASE` configures a subpath deployment. The Go server redirects `/` to the base path when needed, injects the base into the built HTML, serves `/assets/*` with static caching, and returns the SPA entrypoint for non-API unknown routes.

## Operational Notes

- Database connection and migration failures happen during startup and can prevent the backend from serving normally.
- Session/auth failures in the frontend should send users to `/login?reason=<code>` and present operator remediation rather than a misleading login form.
- Distroless production images may not contain a shell; use a temporary client/debug pod for in-cluster database or network troubleshooting.
- Helm v4 is part of the backend dependency graph; build environments must use Go 1.26 or newer.
- Kubectl terminal creates per-session pods only after an admin enables the feature and pre-provisions the `kite-kubectl-admin` ServiceAccount/RBAC.
