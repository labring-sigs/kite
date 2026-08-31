# Environment Variables

Kite supports several environment variables by default to change the default values of some configuration items.

- **KITE_USERNAME**: Set the initial administrator username. The user is created at boot when no users exist yet.
- **KITE_PASSWORD**: Set the initial administrator password. Used together with `KITE_USERNAME`.
- **KUBECONFIG**: Kubernetes configuration file path, default value is `~/.kube/config`. When kite has no configured clusters, it will discover and import clusters from this path at boot.

- **JWT_SECRET**: Secret key used for signing and verifying JWT
- **KITE_ENCRYPT_KEY**: Secret key used for encrypting sensitive data, such as user passwords, OAuth clientSecret, kubeconfig, etc.

- **AUTH_COOKIE_SAMESITE**: SameSite policy for auth cookies. Supported values: `lax`, `strict`, `none`. Default is `lax`. For cross-site iframe embedding (for example Sealos), set this to `none`.
- **AUTH_COOKIE_SECURE**: Secure policy for auth cookies. Supported values: `auto`, `true`, `false`. Default is `auto`. If `AUTH_COOKIE_SAMESITE=none`, this must be `true` and your site must use HTTPS.

- **SEALOS_AUTH_ENABLED**: Enable Sealos login API (`/api/auth/login/sealos`), default is `false`.
- **SEALOS_JWT_SECRET**: Secret used to verify Sealos JWT. Kite uses `HS256` for Sealos token verification.
- **SEALOS_DEFAULT_PROMETHEUS_URL**: Default Prometheus URL for Sealos-managed clusters. If set, Kite writes this value for newly synced Sealos clusters and backfills existing Sealos clusters with empty `prometheus_url` during startup.
- **KITE_NAMESPACE_SCOPE_EXEMPT_NAMESPACES**: Comma-separated namespace allowlist (for example `ns-admin,platform-admin`). For these namespaces, Kite bypasses kubeconfig `current-context.namespace` namespace-scope lock, Sealos SSO auto-roles are granted `*` namespaces on their managed cluster, and those Sealos users are assigned Kite's built-in `admin` role for admin-only app settings such as AI Agent configuration. Use this only for namespaces whose credentials are truly cluster-admin/global.

- **KITE_HELM_ARTIFACT_HUB_ENABLED**: Enable Artifact Hub chart source APIs and UI fallback. Defaults to `true`. Set to `false` for offline deployments.
- **KITE_HELM_OCI_REGISTRY_BASE**: `oci://` registry repository prefix that Kite scans for offline Helm OCI charts, for example `oci://registry.internal/kite-helm`.
- **KITE_HELM_OCI_REPOSITORY_NAME**: Repository name shown in Kite for discovered OCI charts. Defaults to `offline`.
- **KITE_HELM_OCI_DISCOVERY_PAGE_SIZE**: Registry API page size used when listing repositories and tags. Defaults to `100`.
- **KITE_HELM_OCI_DISCOVERY_MAX_REPOSITORIES**: Maximum registry repositories checked while searching the configured prefix. Defaults to `1000`.
- **KITE_HELM_OCI_DISCOVERY_MAX_TAGS_PER_REPOSITORY**: Maximum tags scanned per repository. Defaults to `200`.
- **KITE_HELM_OCI_UPLOAD_MAX_BYTES**: Maximum Helm chart package size accepted by the admin upload API. Defaults to `512MiB`.
- **KITE_HELM_OCI_REGISTRY_PLAIN_HTTP**: Use plain HTTP for the OCI registry API and chart pulls. Defaults to `false`.
- **KITE_HELM_OCI_REGISTRY_INSECURE_SKIP_TLS_VERIFY**: Skip TLS verification for private registry and token endpoints. Defaults to `false`.
- **KITE_HELM_OCI_REGISTRY_CA_FILE**: CA bundle path mounted inside the Kite container for private registry TLS.
- **KITE_HELM_OCI_REGISTRY_USERNAME**: Username used by Kite when listing and pulling OCI chart packages.
- **KITE_HELM_OCI_REGISTRY_PASSWORD**: Password used by Kite when listing and pulling OCI chart packages. Prefer injecting it from a Kubernetes Secret instead of Helm values.
- **KITE_IMAGE_UPLOAD_REGISTRY**: Registry host used by the admin container image archive upload API, for example `registry.internal` or `registry.internal:5000`. Leave empty to reuse `KITE_HELM_OFFLINE_IMAGE_REGISTRY` when configured.
- **KITE_IMAGE_UPLOAD_REPOSITORY_PREFIX**: Repository prefix prepended to uploaded container image archives. Defaults to `kite-images`.
- **KITE_IMAGE_UPLOAD_MAX_BYTES**: Maximum container image archive size accepted by the admin upload API and offline application bundle import API. Defaults to `4GiB`.
- **KITE_IMAGE_UPLOAD_REGISTRY_PLAIN_HTTP**: Use plain HTTP for container image archive uploads. Defaults to `false`.
- **KITE_IMAGE_UPLOAD_REGISTRY_INSECURE_SKIP_TLS_VERIFY**: Skip TLS verification for the container image upload registry and token endpoints. Defaults to `false`.
- **KITE_IMAGE_UPLOAD_REGISTRY_CA_FILE**: CA bundle path mounted inside the Kite container for the container image upload registry TLS.
- **KITE_IMAGE_UPLOAD_REGISTRY_USERNAME**: Username used by Kite when pushing uploaded container image archives.
- **KITE_IMAGE_UPLOAD_REGISTRY_PASSWORD**: Password used by Kite when pushing uploaded container image archives. Prefer injecting it from a Kubernetes Secret instead of Helm values.
- **KITE_HELM_OFFLINE_IMAGES_ENABLED**: Enable offline container image defaults for charts installed from the OCI catalog. Defaults to `false`.
- **KITE_HELM_OFFLINE_IMAGE_REGISTRY**: Registry host used for rendered workload images from offline OCI charts, for example `registry.internal` or `registry.internal:5000`. Do not include `http://`, `https://`, or an `oci://` prefix.
- **KITE_HELM_OFFLINE_IMAGES_ENFORCE**: When offline image defaults are enabled, block installs and upgrades if rendered workload images still point outside `KITE_HELM_OFFLINE_IMAGE_REGISTRY`. Defaults to `true`.

- **HOST**: Used for generating OAuth 2.0 authorization callback addresses, default will be obtained from request headers. If you find the result not as expected, you can manually configure this environment variable.

- **NODE_TERMINAL_IMAGE**: Docker image used for generating Node Terminal Agent.

- **ENABLE_ANALYTICS**: Enable data analytics functionality, default value is `false`. When enabled, Kite will collect limited data to help improve the product.

- **PORT**: Port on which Kite runs, default value is `8080`.
- **DISABLE_CACHE**: Set to `true` to use the direct Kubernetes API client
  instead of the controller-runtime informer cache. `make dev` sets this by
  default for local development to avoid high CPU when cached cluster clients
  repeatedly fail to sync.

Optional frontend environment variables (build-time):

- **VITE_SEALOS_AUTO_LOGIN**: `true` or `false`. Controls whether frontend should auto-attempt Sealos SDK session login. Default is `true`. Sealos auto-login is allowed in both iframe deployments and top-level standalone/local development windows, including use through `sealos-app-dev-bridge`; any SDK availability notice on `/login` is diagnostic-only and does not block the attempt.
