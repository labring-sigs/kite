# Runbook

## Local Development

Install dependencies:

```bash
make deps
```

Run backend and Vite together:

```bash
make dev
```

`make dev` starts the backend with `DISABLE_CACHE=true`. This keeps local
development on the direct Kubernetes API client path instead of starting a
controller-runtime informer cache for every saved cluster. If local CPU usage
spikes, check the backend first:

```bash
ps -Ao pid,ppid,pcpu,pmem,etime,command | sort -nrk3 | head
go tool pprof -top 'http://localhost:6060/debug/pprof/profile?seconds=10'
```

If the profile is dominated by
`sigs.k8s.io/controller-runtime/pkg/manager.(*runnableGroup).Start`, make sure
the process was started through `make dev` or set `DISABLE_CACHE=true` manually
when running `./kite` for local development.

Default local endpoints:

- Backend: `http://localhost:8080`
- Vite dev server: `http://localhost:5173`
- Health check: `http://localhost:8080/healthz`

## Build And Test

Kite uses Helm v4 and requires Go 1.26 or newer for backend builds. Keep local Go, Dockerfile, and CI `GO_VERSION` aligned.

Build frontend static assets and the Go binary:

```bash
make build
```

Run backend tests:

```bash
make test
```

Run lint:

```bash
make lint
```

Run frontend type checks:

```bash
cd ui && pnpm run type-check
```

Run the full frontend production build:

```bash
cd ui && COREPACK_ENABLE_AUTO_PIN=0 pnpm run build
```

When syncing upstream UI features, also scan the touched frontend scopes for
literal i18n keys that are missing from either locale. This one-off command
checks the Helm, AI chat, terminal/log, and overview surfaces:

```bash
node <<'NODE'
const fs = require('fs')
const path = require('path')
function flatten(obj, prefix = '', out = {}) {
  for (const [key, value] of Object.entries(obj)) {
    const next = prefix ? `${prefix}.${key}` : key
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      flatten(value, next, out)
    } else {
      out[next] = value
    }
  }
  return out
}
const en = flatten(JSON.parse(fs.readFileSync('ui/src/i18n/locales/en.json', 'utf8')))
const zh = flatten(JSON.parse(fs.readFileSync('ui/src/i18n/locales/zh.json', 'utf8')))
const files = []
function walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === 'node_modules' || entry.name === 'dist') continue
    const current = path.join(dir, entry.name)
    if (entry.isDirectory()) walk(current)
    else if (/\.(tsx?|jsx?)$/.test(entry.name)) files.push(current)
  }
}
walk('ui/src')
const scopes = {
  helm: /helm/i,
  ai: /ai-chat|aiChat/i,
  terminal: /terminal|log-viewer/i,
  overview: /overview|resource-detail-shell|cronjob-detail/i,
}
const calls = []
const pattern = /\bt\(\s*(['"])([A-Za-z0-9_.-]+)\1/g
for (const file of files) {
  const source = fs.readFileSync(file, 'utf8')
  let match
  while ((match = pattern.exec(source))) {
    calls.push({ file, key: match[2] })
  }
}
let missing = 0
for (const [name, scope] of Object.entries(scopes)) {
  const scoped = calls.filter(({ file, key }) => scope.test(file) || scope.test(key))
  const unique = new Map()
  for (const call of scoped) {
    if (!(call.key in en) || !(call.key in zh)) {
      unique.set(`${call.key}:${call.file}`, call)
    }
  }
  console.log(`${name}: ${unique.size}`)
  missing += unique.size
}
process.exit(missing === 0 ? 0 : 1)
NODE
```

Build documentation:

```bash
make docs-build
```

## Important Environment Variables

- `PORT`: HTTP listen port, default `8080`.
- `KITE_BASE`: optional subpath base for deployments such as `/kite`.
- `DB_TYPE`: one of `sqlite`, `mysql`, or `postgres`.
- `DB_DSN`: database DSN. Defaults to `dev.db` for SQLite.
- `DB_AUTO_CREATE`: whether Kite should create missing MySQL/PostgreSQL databases before migration. Defaults to `true`.
- `JWT_SECRET`: signing secret for Kite sessions.
- `KITE_ENCRYPT_KEY`: encryption key for sensitive stored values.
- `KUBECONFIG`: source path for first-run cluster import when no clusters exist.
- `SEALOS_AUTH_ENABLED`: enables the Sealos login API.
- `SEALOS_JWT_SECRET`: JWT secret used for Sealos auth validation.
- `KITE_NAMESPACE_SCOPE_EXEMPT_NAMESPACES`: comma-separated Sealos workspace namespaces that represent global/admin credentials. Matching Sealos users receive `*` namespaces on their managed cluster and Kite's built-in `admin` role. When the same Sealos user logs into a non-exempt workspace, Kite removes that user's stale built-in `admin` assignment before RBAC sync.
- `AUTH_COOKIE_SAMESITE` and `AUTH_COOKIE_SECURE`: cookie settings for normal and iframe deployments.
- `VITE_SEALOS_AUTO_LOGIN`: frontend build-time flag controlling Sealos SDK session login attempts.

AI and terminal feature state is stored in the admin general settings record rather than plain environment variables:

- `aiAgentEnabled`, `aiProvider`, `aiModel`, `aiApiKey`, `aiBaseUrl`, and `aiMaxTokens` configure the AI assistant. New installs enable the AI Agent UI by default, but chat requests still require a configured API key.
- `kubectlEnabled`, `kubectlImage`, and `nodeTerminalImage` configure optional terminal helper pods.

Sealos auth notes:

- Kite does not block Sealos auth solely because the app is opened as the top-level window. Standalone local development and `sealos-app-dev-bridge` can still attempt Sealos SDK auto-login when `SEALOS_AUTH_ENABLED=true`.
- `/login` can show a non-blocking Sealos SDK availability notice when the SDK session channel is unavailable. Treat it as a diagnostic hint for Sealos Desktop or `sealos-app-dev-bridge`, not as an access gate.
- The display name `admin` is not enough for Kite admin-only settings. Check `/api/auth/user` or the `role_assignments` table: the user must have the built-in `admin` role. For Sealos workspaces in `KITE_NAMESPACE_SCOPE_EXEMPT_NAMESPACES`, the next Sealos login/sync assigns that role automatically; for non-exempt workspaces, the next Sealos login/sync removes stale built-in `admin` assignments for that Sealos username.
- Non-exempt Sealos kubeconfigs with a current-context namespace are loaded as namespace-scoped clients. Kite disables the controller-runtime informer cache for those clients, so direct Kubernetes API reads are expected even when the global process cache is enabled.
- If Sealos auto-login fails in standalone/local development, verify the bridge or Sealos Desktop session first, then inspect `/api/auth/login/sealos` responses and backend logs.

## Auth Fault Page

`/login` is now an operational fault page. If a customer sees it, treat it as a signal to inspect server-side state instead of asking them to sign in manually.

Common reason codes:

- `unauthenticated`: frontend could not find a valid session.
- `session_refresh_failed`: cookie/session refresh failed.
- `authentication_failed`: an API auth retry failed.
- `insufficient_permissions`: the authenticated user has no matching Kite RBAC access.
- `token_exchange_failed`, `user_info_failed`, `jwt_generation_failed`, `callback_failed`, `callback_error`: OAuth callback or session creation failed.
- `user_disabled`: the Kite user exists but is disabled.

Suggested checks:

1. Inspect backend logs for database, migration, JWT, OAuth, Sealos auth, or session creation errors.
2. Verify `DB_TYPE`, `DB_DSN`, and database account permissions. If `DB_AUTO_CREATE=true`, the account needs database creation permission for MySQL/PostgreSQL.
3. Verify `JWT_SECRET`, `KITE_ENCRYPT_KEY`, `SEALOS_AUTH_ENABLED`, `SEALOS_JWT_SECRET`, and OAuth provider callback settings.
4. Confirm the browser receives and sends the `auth_token` cookie. For iframe/cross-site deployments, check `AUTH_COOKIE_SAMESITE=none`, `AUTH_COOKIE_SECURE=true`, and HTTPS.
5. For `insufficient_permissions`, check Kite RBAC role assignments and OAuth group/user mapping.

## Database Troubleshooting

Startup database failures usually happen before the full UI is usable. Look for panic messages such as:

- `failed to ensure database exists`
- `failed to connect database`
- `failed to migrate database`
- `database connection is nil`

For SQLite hostPath issues, see `docs/faq.md`. For production persistence, prefer MySQL or PostgreSQL.

## Kubernetes Connectivity Troubleshooting

- If managed Kubernetes kubeconfigs use an `exec` plugin such as `aws`, `gcloud`, or `kubelogin`, use a Service Account token kubeconfig instead. See `docs/config/managed-k8s-auth.md`.
- If resource access fails with permission errors, verify Kite RBAC first, then Kubernetes RBAC for the service account or imported kubeconfig.
- If a Sealos personal workspace stays on a loading spinner while the backend logs repeat `failed to wait for cache sync`, inspect the kubeconfig current-context namespace. Non-exempt namespace-scoped kubeconfigs should use direct Kubernetes API reads instead of the controller-runtime informer cache; backend cluster build failures are rate-limited, and a new Sealos login or cluster config update triggers an immediate retry.
- If a production image has no shell, use a temporary debug/client pod rather than `kubectl exec` into Kite.

## AI Assistant Operations

- The AI assistant UI is enabled by default for new installs. The visible settings entry is the configure button in the AI chat panel; only Kite admins can edit the Settings page, which only shows AI Agent settings.
- Chat requests require an OpenAI-compatible or Anthropic-compatible provider configuration with an API key. If the API key is missing, the runtime treats AI as not enabled.
- Read-only tools still use the current authenticated user, cluster, and namespace scope.
- In namespace-scoped non-exempt Sealos workspaces, AI tools resolve omitted namespace and `_all` to the current workspace namespace, reject ordinary cluster-scoped resources such as Nodes/Namespaces, hide arbitrary Prometheus queries, and show namespace-local cluster overview data only.
- Mutating tools require both Kite RBAC and an explicit continue/confirmation step. Pending sessions are scoped to the same user and cluster.
- Helm actions in AI chat are structured tool calls backed by Kite's Helm SDK integration, not direct `helm` shell commands inside the Kite pod. The pod does not need a Helm CLI binary for AI Helm workflows.
- AI Helm install and upgrade should run the matching dry-run tool first. The dry-run response summarizes rendered resources for review; the actual install, upgrade, rollback, or uninstall tool then enters the same confirmation flow as other mutating tools.
- AI Helm tools require `helmreleases` RBAC for the release action and rendered-resource RBAC through `pkg/helmguard`. If a tool fails with a permission error, check both the `helmreleases` verb on the target namespace and the rendered Kubernetes resources in the dry-run summary.
- If AI chat returns an AI Agent configuration, disabled, or provider error, check the general settings record, provider base URL, API key, model name, and whether the current user has Kite's built-in `admin` role when trying to edit `/settings`.

## Helm Operations

- Chart catalog read APIs live under `/api/v1/charts` for authenticated users. Repository create/delete and catalog management APIs remain admin-only under `/api/v1/admin/charts`; stored repository credentials must not be exposed in responses.
- Offline environments can disable Artifact Hub with `KITE_HELM_ARTIFACT_HUB_ENABLED=false` or `helmCatalog.artifactHub.enabled=false`. This disables the backend Artifact Hub proxy endpoints and the frontend fallback.
- Kite discovers offline Helm OCI charts by scanning only the configured registry repository prefix (`helmCatalog.oci.base` / `KITE_HELM_OCI_REGISTRY_BASE`). It does not expose arbitrary registry contents outside that prefix.
- Registry credentials and TLS options are server-side only. Chart read APIs return clean `oci://host/prefix/chart:version` URLs without credentials, query parameters, or fragments. Helm OCI tags encode SemVer build metadata by replacing `+` with `_`.
- If OCI discovery returns `401 Unauthorized`, first confirm the username/password Secret is injected, then inspect the registry's `WWW-Authenticate` challenge. A direct Basic request can still return 401 when the registry requires a Bearer token exchange; Kite supports anonymous, Basic, and Bearer challenge flows. For Bearer failures, also verify the advertised token realm is reachable and that `caFile` or `insecureSkipTLSVerify` covers its HTTPS certificate, including when the registry API uses `plainHTTP`.
- Offline chart artifacts and container images can share one registry host, but they use different repository paths. Keep charts under a dedicated prefix such as `oci://registry.internal/kite-helm/<chart>:<version>` and container images under their normal repositories such as `registry.internal/bitnami/nginx:<tag>`.
- The admin UI transfers offline apps as `.kiteapp.tar.gz` bundles, not as separate chart and image uploads. A bundle contains `kite-bundle.json`, one or more Helm chart archives, and the container image archives required by those charts after offline image values are rendered. Import validates the bundle, pushes all required images first, and pushes the chart last so a chart is not exposed in the catalog without its images. The UI starts import as a background job after the bundle upload is accepted, polls job status outside the dialog, and lets users keep working while registry pushes continue. Import job status is in-memory and may be lost if the Kite pod restarts. Export packages selected OCI catalog charts with their rendered workload images so the result can be imported into another configured Kite cluster.
- The lower-level backend upload primitives remain separate for compatibility and internal use. Helm chart packages use `POST /api/v1/admin/charts/oci/upload` and are pushed under `helmCatalog.oci.base`; container image archives use `POST /api/v1/admin/images/upload` and are pushed under `helmCatalog.imageUploads.registry` plus `helmCatalog.imageUploads.repositoryPrefix`. Bundle import/export is exposed under `POST /api/v1/admin/charts/offline-bundles/import-jobs` plus `GET /api/v1/admin/charts/offline-bundles/import-jobs/:id` for the background import flow, the legacy synchronous `POST /api/v1/admin/charts/offline-bundles/import` route for compatibility, and `POST /api/v1/admin/charts/offline-bundles/export` for export.
- Minimal OCI discovery example:

```yaml
helmCatalog:
  artifactHub:
    enabled: false
  oci:
    base: oci://registry.internal/kite-helm
    repositoryName: offline
    plainHTTP: true
    insecureSkipTLSVerify: true
    username: admin
    passwordSecretName: registry-credentials
    passwordSecretKey: KITE_HELM_OCI_REGISTRY_PASSWORD
    uploadMaxBytes: 512MiB
  imageUploads:
    registry: registry.internal
    repositoryPrefix: kite-images
    maxBytes: 4GiB
    plainHTTP: true
    insecureSkipTLSVerify: true
    username: admin
    passwordSecretName: registry-credentials
    passwordSecretKey: KITE_IMAGE_UPLOAD_REGISTRY_PASSWORD
  offlineImages:
    enabled: true
    registry: registry.internal
    enforce: true
```

If `helmCatalog.imageUploads.registry` is empty, Kite reuses
`helmCatalog.offlineImages.registry` as the image upload registry when that
offline image registry is configured. Use separate Secret keys when the chart
OCI registry and image upload registry credentials differ. The upload limits are
rendered as `KITE_HELM_OCI_UPLOAD_MAX_BYTES` and
`KITE_IMAGE_UPLOAD_MAX_BYTES`; keep any ingress or gateway body-size limit at
least as high as the server-side limit. The same image archive limit bounds
offline bundle uploads.

- If you are preparing charts without Kite bundle export, mirror their rendered
workload images before installing an offline OCI chart:

```bash
scripts/mirror-helm-chart-images.sh \
  --chart oci://registry.internal/kite-helm/nginx \
  --version 25.0.12 \
  --registry registry.internal
```

Use `--dry-run` first to inspect the copy plan. The script renders the chart
twice, maps each original image to the image rendered with the offline registry
by Kubernetes resource and container identity, deduplicates the copy plan, and
uses `crane copy` for the actual transfer. It
also injects `global.security.allowInsecureImages=true` for target rendering so
Bitnami-style image verification does not block offline registry substitution.

- When `helmCatalog.offlineImages.enabled=true`, Kite applies the same offline
image defaults to OCI catalog installs, upgrades, AI Helm tools, and scheduled
auto-upgrades. It injects `global.imageRegistry` and
`global.security.allowInsecureImages` only when the user did not explicitly set
them, then dry-runs the release and checks rendered Pod, Deployment,
StatefulSet, DaemonSet, ReplicaSet, ReplicationController, Job, and CronJob
container images. If `helmCatalog.offlineImages.enforce=true`, any rendered
image outside `helmCatalog.offlineImages.registry` blocks the write before
cluster state changes. Kite persists the chart source in Helm chart metadata on
install or explicit chart upgrades, so later current-chart upgrades continue to
apply the OCI offline image policy.

- Helm release APIs use the canonical `helmreleases` resource path. The legacy `helmrelease` route remains for compatibility.
- Helm install, upgrade, rollback, uninstall, and auto-upgrade render target manifests before writing. Added resources require `create`, retained resources require `update`, and removed resources require `delete` on the rendered Kubernetes resource.
- The AI assistant uses the same Helm SDK and rendered-manifest guard path as the HTTP Helm release API. Do not troubleshoot AI Helm failures by installing a shell or Helm CLI into the Kite container unless a separate debug session explicitly needs those tools.
- Cluster-scoped rendered resources such as CRDs, Namespaces, ClusterRoles, and ClusterRoleBindings require the Kite admin role and are rejected on namespace-scoped Sealos clusters.
- If an upgrade or rollback fails with a rendered resource permission error, inspect the dry-run manifest diff and grant the specific resource verb in Kite RBAC or choose a chart/values set that stays inside the user's namespace scope.

## Sealos Packaging

- The Sealos deploy entrypoint loads platform helpers from `/root/.sealos/cloud/scripts/tools.sh`, reads global HTTP/TLS settings, and then passes `cloudDomain`, `cloudPort`, `httpPort`, `disableHttps`, `certSecretName`, `platform.tlsRejectUnauthorized`, and `db.postgres.native.kubeblocksVersion` to Helm.
- App override values are loaded from `/root/.sealos/cloud/values/apps/kite/*-values.yaml` in sorted order. If that directory has no `*-values.yaml`, the entrypoint copies the chart default `kite-values.yaml` there before running Helm.
- Release workflows generate `.tar.gz.md5` files for Sealos image packages and attach them to GitHub artifacts/releases. They intentionally do not upload packages or checksums to OSS.

## Kubectl Terminal Operations

- Kubectl terminal is disabled by default and is admin-only.
- Before enabling it, create a ServiceAccount named `kite-kubectl-admin` in the agent namespace with exactly the Kubernetes RBAC the deployment intends to expose.
- Kite creates and cleans up per-session kubectl pods, but it must not auto-create cluster-admin bindings.
- If the terminal fails immediately, check the general settings flag, the configured kubectl image, and whether the `kite-kubectl-admin` ServiceAccount exists in the agent namespace.

## Production Image Notes

For production deployments, build and publish `linux/amd64` images by default unless ARM is explicitly requested.
