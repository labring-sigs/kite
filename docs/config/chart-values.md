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
| `anonymousUserEnabled` | Enable anonymous user access with full admin privileges. Use with caution in production. | `false`                                              |
| `jwtSecret`            | Secret key used for signing JWT tokens. Change this in production.                       | `"kite-default-jwt-secret-key-change-in-production"` |
| `encryptKey`           | Secret key used for encrypting sensitive data. Change this in production.                | `"kite-default-encryption-key-change-in-production"` |
| `host`                 | Hostname for the application                                                             | `""`                                                 |
| `cloudDomain`          | Sealos cloud domain. Used to render ingress/app host                                     | `"127.0.0.1.nip.io"`                                |
| `sealos.jwtSecret`     | Value injected to env `SEALOS_JWT_SECRET`                                                | `""`                                                 |

## Database Configuration

| Parameter | Description                                                              | Default  |
| --------- | ------------------------------------------------------------------------ | -------- |
| `db.type` | Database type: `sqlite`, `postgres`, `mysql`                             | `postgres` |
| `db.dsn`  | Full DSN string for MySQL/Postgres. Required when type is mysql/postgres | `""`     |

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

## Sealos App Configuration

| Parameter     | Description                                    | Default |
| ------------- | ---------------------------------------------- | ------- |
| `app.enabled` | Whether to create `app.sealos.io/v1 App`      | `false` |

App metadata is intentionally fixed in templates:
- namespace: `app-system`
- name: `kite`
- type/displayType: `iframe` / `normal`
- icon/url: `https://kite.<cloudDomain>/logo.svg` and `https://kite.<cloudDomain>`

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

Ingress behavior is fixed in templates:
- host: `kite.<cloudDomain>`
- className: `nginx`
- path/pathType: `/` / `Prefix`
- TLS: enabled, secret name `wildcard-cert`
- annotations:
  - `nginx.ingress.kubernetes.io/proxy-read-timeout: '3600'`
  - `nginx.ingress.kubernetes.io/proxy-send-timeout: '3600'`

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
