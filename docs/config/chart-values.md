# Chart Values

This document describes all available configuration options for the Kite Helm Chart.

## Basic Configuration

| Parameter          | Description                                                | Default               |
| ------------------ | ---------------------------------------------------------- | --------------------- |
| `replicaCount`     | Number of replicas                                         | `1`                   |
| `image.repository` | Container image repository                                 | `crpi-7jr40k6elhldekqp.cn-hangzhou.personal.cr.aliyuncs.com/mlhiter/kite` |
| `image.pullPolicy` | Image pull policy                                          | `IfNotPresent`        |
| `image.tag`        | Image tag. If set, will override the chart's `appVersion`. | `"1.0.8"`             |
| `imagePullSecrets` | Image pull secrets for private repositories                | `[]`                  |
| `nameOverride`     | Override chart name                                        | `""`                  |
| `fullnameOverride` | Override full name                                         | `""`                  |
| `debug`            | Enable debug mode                                          | `false`               |
| `basePath`         | Base path where Kite is served. See notes below. | `""`                 |

## Authentication & Security

| Parameter              | Description                                                                              | Default                                              |
| ---------------------- | ---------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| `jwtSecret`            | Secret key used for signing JWT tokens. Change this in production.                       | `"kite-default-jwt-secret-key-change-in-production"` |
| `encryptKey`           | Secret key used for encrypting sensitive data. Change this in production.                | `"kite-default-encryption-key-change-in-production"` |
| `host`                 | Hostname for the application                                                             | `""`                                                 |
| `cloudDomain`          | Sealos cloud domain. Used to render ingress/app host                                     | `"127.0.0.1.nip.io"`                                |
| `cloudPort`            | External HTTPS port used when rendering Sealos app URLs                                  | `"443"`                                             |
| `httpPort`             | External HTTP port used when `disableHttps=true`                                         | `"80"`                                              |
| `disableHttps`         | Render external Sealos app URLs and Ingress without HTTPS/TLS                            | `false`                                             |
| `certSecretName`       | Ingress TLS secret used when HTTPS is enabled                                            | `"wildcard-cert"`                                   |
| `platform.tlsRejectUnauthorized` | Value injected to env `NODE_TLS_REJECT_UNAUTHORIZED`                          | `"1"`                                               |
| `sealos.jwtSecret`     | Value injected to env `SEALOS_JWT_SECRET`                                                | `""`                                                 |

## Database Configuration

| Parameter       | Description                                                                                                  | Default    |
| --------------- | ------------------------------------------------------------------------------------------------------------ | ---------- |
| `db.type`       | Database type: `sqlite`, `postgres`, `mysql`                                                                 | `postgres` |
| `db.dsn`        | Full DSN string for MySQL/Postgres. Required when type is mysql/postgres and native Postgres is disabled      | `""`       |
| `db.autoCreate` | Whether Kite should create the target MySQL/Postgres database automatically before running schema migrations | `true`     |
| `db.postgres.native.kubeblocksVersion` | Sealos global KubeBlocks version marker read by the deploy entrypoint | `"0.8.2"` |

When `db.autoCreate` is enabled, the configured database user must have permission to create databases. Kite only creates the target database itself; tables are still created by the normal application migration flow.

### SQLite Configuration

| Parameter                                 | Description                                               | Default             |
| ----------------------------------------- | --------------------------------------------------------- | ------------------- |
| `db.sqlite.persistence.pvc.enabled`       | Whether to create a PVC to store the sqlite database file | `false`             |
| `db.sqlite.persistence.pvc.existingClaim` | Use existing PVC                                          | `""`                |
| `db.sqlite.persistence.pvc.storageClass`  | StorageClass for PVC (optional)                           | `""`                |
| `db.sqlite.persistence.pvc.accessModes`   | Access modes for PVC                                      | `["ReadWriteOnce"]` |
| `db.sqlite.persistence.pvc.size`          | Requested storage size for PVC                            | `1Gi`               |
| `db.sqlite.persistence.hostPath.enabled`  | Whether to use hostPath storage                           | `false`             |
| `db.sqlite.persistence.hostPath.path`     | hostPath path                                             | `/path/to/host/dir` |
| `db.sqlite.persistence.hostPath.type`     | hostPath type                                             | `DirectoryOrCreate` |
| `db.sqlite.persistence.mountPath`         | Mount path inside container                               | `/data`             |
| `db.sqlite.persistence.filename`          | SQLite filename inside mountPath                          | `kite.db`           |

## Environment Variables

| Parameter   | Description                              | Default |
| ----------- | ---------------------------------------- | ------- |
| `extraEnvs` | List of additional environment variables | `[]`    |

## Helm Chart Catalog

| Parameter                                      | Description                                                             | Default   |
| ---------------------------------------------- | ----------------------------------------------------------------------- | --------- |
| `helmCatalog.artifactHub.enabled`              | Enable Artifact Hub chart search/detail proxy APIs                      | `true`    |
| `helmCatalog.oci.base`                         | `oci://` registry repository prefix scanned for offline Helm OCI charts | `""`      |
| `helmCatalog.oci.repositoryName`               | Repository name shown in Kite for discovered OCI charts                 | `offline` |
| `helmCatalog.oci.discoveryPageSize`            | Registry API page size used when listing repositories and tags          | `100`     |
| `helmCatalog.oci.discoveryMaxRepositories`     | Maximum registry repositories checked while searching the configured prefix | `1000` |
| `helmCatalog.oci.discoveryMaxTagsPerRepository` | Maximum tags scanned per repository                                    | `200`     |
| `helmCatalog.oci.uploadMaxBytes`               | Maximum Helm chart package size accepted by the admin upload API        | `512MiB`  |
| `helmCatalog.oci.plainHTTP`                    | Use plain HTTP for OCI registry API and chart pulls                     | `false`   |
| `helmCatalog.oci.insecureSkipTLSVerify`        | Skip TLS verification for private registry and token endpoints          | `false`   |
| `helmCatalog.oci.caFile`                       | CA bundle path mounted inside the Kite container for private registry TLS | `""`    |
| `helmCatalog.oci.username`                     | Username used by Kite when listing and pulling OCI chart packages       | `""`      |
| `helmCatalog.oci.password`                     | Password stored in the Kite Secret and used for OCI registry access; prefer `passwordSecretName` for production installs | `""` |
| `helmCatalog.oci.passwordSecretName`           | Existing Secret containing the OCI registry password                    | `""`      |
| `helmCatalog.oci.passwordSecretKey`            | Key inside `passwordSecretName` used as `KITE_HELM_OCI_REGISTRY_PASSWORD` | `KITE_HELM_OCI_REGISTRY_PASSWORD` |
| `helmCatalog.imageUploads.registry`            | Registry host for uploaded container image archives; empty reuses `helmCatalog.offlineImages.registry` when set | `""` |
| `helmCatalog.imageUploads.repositoryPrefix`    | Repository prefix prepended to uploaded container image archives        | `kite-images` |
| `helmCatalog.imageUploads.maxBytes`            | Maximum container image archive or offline application bundle size accepted by the admin upload/import APIs | `4GiB` |
| `helmCatalog.imageUploads.plainHTTP`           | Use plain HTTP for container image archive uploads                      | `false`   |
| `helmCatalog.imageUploads.insecureSkipTLSVerify` | Skip TLS verification for the container image upload registry and token endpoints | `false` |
| `helmCatalog.imageUploads.caFile`              | CA bundle path mounted inside the Kite container for container image upload registry TLS | `""` |
| `helmCatalog.imageUploads.username`            | Username used by Kite when pushing uploaded container image archives    | `""`      |
| `helmCatalog.imageUploads.password`            | Password stored in the Kite Secret for container image archive uploads; prefer `passwordSecretName` for production installs | `""` |
| `helmCatalog.imageUploads.passwordSecretName`  | Existing Secret containing the container image upload registry password | `""`      |
| `helmCatalog.imageUploads.passwordSecretKey`   | Key inside `passwordSecretName` used as `KITE_IMAGE_UPLOAD_REGISTRY_PASSWORD` | `KITE_IMAGE_UPLOAD_REGISTRY_PASSWORD` |
| `helmCatalog.offlineImages.enabled`            | Inject offline container image defaults for OCI catalog installs/upgrades | `false` |
| `helmCatalog.offlineImages.registry`           | Registry host expected in rendered workload images from offline OCI charts | `""` |
| `helmCatalog.offlineImages.enforce`            | Block OCI chart installs/upgrades when rendered workload images still point outside the offline registry | `true` |

Offline deployments can disable Artifact Hub and expose a local OCI-backed
chart catalog without a Helm `index.yaml`. Kite scans only the configured
repository prefix:

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

If the registry contains `kite-helm/demo-chart` tags `0.1.0` and `0.2.0`, Kite
resolves those entries as `oci://registry.internal/kite-helm/demo-chart:0.2.0`
for browsing, install, upgrade, and auto-upgrade.

The configured `base` must be an `oci://` URL with a repository prefix and must
not include credentials, tokens, query parameters, fragments, tags, or digests.
Use `helmCatalog.oci.username` plus `helmCatalog.oci.passwordSecretName` for
private registries in production; `helmCatalog.oci.password` remains available
for simple installs and is copied into the chart-managed Secret. Credentials are
not part of catalog API responses. For self-signed registries, either mount a CA
bundle and set `helmCatalog.oci.caFile`, or set
`helmCatalog.oci.insecureSkipTLSVerify` in trusted offline environments. Some
private registries expose chart manifests over HTTP but advertise an HTTPS token
realm, so `plainHTTP` and `insecureSkipTLSVerify` may need to be enabled
together. Helm OCI tag encoding is preserved (`+` in SemVer build metadata
becomes `_` in the registry tag).

Kite's admin UI transfers offline apps as `.kiteapp.tar.gz` bundles. A bundle
binds Helm chart archives to the rendered workload image archives required for
offline install, can include multiple apps, and can be exported from one
configured Kite cluster and imported into another. Import validates the bundle,
pushes required images first, and pushes the chart last so the OCI catalog does
not expose a chart whose images are missing. The UI starts imports as
background jobs after the bundle upload is accepted; job status is in-memory and
may be lost if the Kite pod restarts.

Kite still keeps the lower-level backend flows separate. Helm chart packages
(`.tgz`) are pushed through the chart upload API into the configured
`helmCatalog.oci.base` prefix and then appear in the OCI chart catalog.
Container image archives are pushed through a separate image upload API into
`helmCatalog.imageUploads.registry` plus
`helmCatalog.imageUploads.repositoryPrefix`; they do not become chart catalog
entries. If `helmCatalog.imageUploads.registry` is empty, Kite falls back to
`helmCatalog.offlineImages.registry` when that offline image registry is set.
Use a separate Secret key for `KITE_IMAGE_UPLOAD_REGISTRY_PASSWORD` when the
chart and image registry credentials differ. Bundle uploads are bounded by the
image upload size limit because they may contain container image archives.

`helmCatalog.offlineImages` is separate from OCI chart discovery. Keep Helm OCI
chart artifacts under the configured chart prefix such as
`oci://registry.internal/kite-helm/nginx:25.0.12`, and mirror container images
into the same registry host using their normal repository paths such as
`registry.internal/bitnami/nginx:latest`. For OCI catalog charts, Kite injects
`global.imageRegistry=<registry>` and
`global.security.allowInsecureImages=true` when those values are absent, renders
the release, and blocks the write if rendered workload images still reference an
external registry. Kite records the chart source on installed releases, so
current-chart upgrades of releases installed from the OCI catalog keep the same
offline image policy even when the client omits `source`. Kite does not copy
container images during install; prepare them with offline bundle import,
`scripts/mirror-helm-chart-images.sh`, or an equivalent registry sync process.

## Sealos App Configuration

| Parameter     | Description                                    | Default |
| ------------- | ---------------------------------------------- | ------- |
| `app.enabled` | Whether to create `app.sealos.io/v1 App`      | `false` |

App metadata is intentionally fixed in templates:
- namespace: `app-system`
- name: `kite`
- type/displayType: `iframe` / `normal`
- icon/url: rendered from `disableHttps`, `cloudPort`, and `httpPort`.

## Service Account Configuration

| Parameter                    | Description                                         | Default |
| ---------------------------- | --------------------------------------------------- | ------- |
| `serviceAccount.create`      | Whether to create a service account                 | `true`  |
| `serviceAccount.automount`   | Automatically mount service account API credentials | `true`  |
| `serviceAccount.annotations` | Annotations for service account                     | `{}`    |
| `serviceAccount.name`        | Name of service account to use                      | `""`    |

## RBAC Configuration

| Parameter     | Description                      | Default           |
| ------------- | -------------------------------- | ----------------- |
| `rbac.create` | Whether to create RBAC resources | `true`            |
| `rbac.rules`  | List of RBAC rules               | See example below |

### RBAC Rules Example

```yaml
rbac:
  rules:
    - apiGroups: ["*"]
      resources: ["*"]
      verbs: ["*"]
    - nonResourceURLs: ["*"]
      verbs: ["*"]
```

## Pod Configuration

| Parameter            | Description                    | Default |
| -------------------- | ------------------------------ | ------- |
| `podAnnotations`     | Kubernetes annotations for Pod | `{}`    |
| `podLabels`          | Kubernetes labels for Pod      | `{}`    |
| `podSecurityContext` | Pod security context           | `{}`    |
| `securityContext`    | Container security context     | `{}`    |

## Service Configuration

| Parameter      | Description  | Default     |
| -------------- | ------------ | ----------- |
| `service.type` | Service type | `ClusterIP` |
| `service.port` | Service port | `8080`      |

## Ingress Configuration

| Parameter             | Description                | Default           |
| --------------------- | -------------------------- | ----------------- |
| `ingress.enabled`     | Whether to enable Ingress  | `true`            |
| `ingress.proxyBodySize` | NGINX body size limit for repository uploads; keep at least as large as the largest upload limit | `4g` |

Ingress behavior is fixed in templates:
- host: `kite.<cloudDomain>`
- className: `nginx`
- path/pathType: `/` / `Prefix`
- TLS: rendered only when `disableHttps=false`; secret name comes from `certSecretName`
- annotations:
  - `nginx.ingress.kubernetes.io/proxy-read-timeout: '3600'`
  - `nginx.ingress.kubernetes.io/proxy-send-timeout: '3600'`
  - `nginx.ingress.kubernetes.io/proxy-body-size: <ingress.proxyBodySize>`

### Ingress Example

```yaml
ingress:
  enabled: true
cloudDomain: 192.168.10.70.nip.io
```

## Resource Limits

| Parameter   | Description                            | Default |
| ----------- | -------------------------------------- | ------- |
| `resources` | Container resource limits and requests | `{}`    |

### Resource Limits Example

```yaml
resources:
  limits:
    cpu: 100m
    memory: 128Mi
  requests:
    cpu: 100m
    memory: 128Mi
```

## Health Checks

| Parameter        | Description                   | Default           |
| ---------------- | ----------------------------- | ----------------- |
| `livenessProbe`  | Liveness probe configuration  | See example below |
| `readinessProbe` | Readiness probe configuration | See example below |

### Health Check Example

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: http
  initialDelaySeconds: 10
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /healthz
    port: http
  initialDelaySeconds: 10
  periodSeconds: 10
```

## Storage Configuration

| Parameter      | Description                            | Default |
| -------------- | -------------------------------------- | ------- |
| `volumes`      | Additional volume configurations       | `[]`    |
| `volumeMounts` | Additional volume mount configurations | `[]`    |

## Scheduling Configuration

| Parameter      | Description               | Default |
| -------------- | ------------------------- | ------- |
| `nodeSelector` | Node selector             | `{}`    |
| `tolerations`  | Tolerations configuration | `[]`    |
| `affinity`     | Affinity configuration    | `{}`    |
