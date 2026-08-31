# 运行手册

## 本地开发

安装依赖：

```bash
make deps
```

同时启动后端和 Vite：

```bash
make dev
```

`make dev` 启动后端时会带上 `DISABLE_CACHE=true`。本地开发默认走直接
Kubernetes API client 路径，不为每个已保存集群启动 controller-runtime
informer cache。若本地 CPU 飙高，先查后端进程：

```bash
ps -Ao pid,ppid,pcpu,pmem,etime,command | sort -nrk3 | head
go tool pprof -top 'http://localhost:6060/debug/pprof/profile?seconds=10'
```

如果 profile 主要落在
`sigs.k8s.io/controller-runtime/pkg/manager.(*runnableGroup).Start`，确认进程是
通过 `make dev` 启动的，或本地手动运行 `./kite` 时显式设置
`DISABLE_CACHE=true`。

默认本地入口：

- 后端：`http://localhost:8080`
- Vite 开发服务：`http://localhost:5173`
- 健康检查：`http://localhost:8080/healthz`

## 构建与测试

构建前端静态资源和 Go 二进制：

```bash
make build
```

运行后端测试：

```bash
make test
```

运行 lint：

```bash
make lint
```

运行前端类型检查：

```bash
cd ui && pnpm run type-check
```

构建文档：

```bash
make docs-build
```

## 关键环境变量

- `PORT`：HTTP 监听端口，默认 `8080`。
- `KITE_BASE`：部署在子路径时使用，例如 `/kite`。
- `DB_TYPE`：`sqlite`、`mysql` 或 `postgres`。
- `DB_DSN`：数据库 DSN。SQLite 默认是 `dev.db`。
- `DB_AUTO_CREATE`：是否在迁移前自动创建缺失的 MySQL/PostgreSQL database，默认 `true`。
- `JWT_SECRET`：Kite 会话签名密钥。
- `KITE_ENCRYPT_KEY`：敏感存储值的加密密钥。
- `KUBECONFIG`：首次导入集群时使用的 kubeconfig 路径。
- `SEALOS_AUTH_ENABLED`：启用 Sealos 登录接口。
- `SEALOS_JWT_SECRET`：Sealos 认证校验使用的 JWT 密钥。
- `KITE_NAMESPACE_SCOPE_EXEMPT_NAMESPACES`：逗号分隔的 Sealos 工作空间命名空间列表，表示这些工作空间使用全局/管理员凭据。命中的 Sealos 用户会获得其托管集群下的 `*` 命名空间权限，并被分配 Kite 内置 `admin` 角色。
- `AUTH_COOKIE_SAMESITE` 和 `AUTH_COOKIE_SECURE`：普通部署和 iframe 部署下的 Cookie 策略。
- `VITE_SEALOS_AUTO_LOGIN`：构建期前端开关，控制是否自动尝试 Sealos SDK 会话登录。

AI 和终端类功能状态存储在管理员通用设置记录中，而不是普通环境变量：

- `aiAgentEnabled`、`aiProvider`、`aiModel`、`aiApiKey`、`aiBaseUrl` 和 `aiMaxTokens` 配置 AI 助手。新安装默认显示并启用 AI Agent UI，但真正发起聊天请求仍需要配置 API Key。
- `kubectlEnabled`、`kubectlImage` 和 `nodeTerminalImage` 配置可选终端辅助 Pod。

Sealos 认证说明：

- Kite 不会仅因为应用以顶层窗口打开而阻断 Sealos 认证。`SEALOS_AUTH_ENABLED=true` 时，独立本地开发页面和 `sealos-app-dev-bridge` 仍会优先尝试 Sealos SDK 自动登录。
- `/login` 可以在 SDK 会话通道不可用时展示非阻断的 Sealos SDK 可用性提示。它只是 Sealos Desktop 或 `sealos-app-dev-bridge` 的诊断线索，不是访问闸门。
- 显示名是 `admin` 不等于拥有 Kite 管理员权限。需要检查 `/api/auth/user` 或 `role_assignments` 表，确认当前用户拥有内置 `admin` 角色。对于命中 `KITE_NAMESPACE_SCOPE_EXEMPT_NAMESPACES` 的 Sealos 工作空间，下一次 Sealos 登录/同步会自动补齐该角色。
- 如果独立本地开发中的 Sealos 自动登录失败，先检查 bridge 或 Sealos Desktop 会话，再查看 `/api/auth/login/sealos` 响应和后端日志。

## 认证故障页

`/login` 现在是运维故障页。客户看到该页时，应优先检查服务端状态，而不是要求客户手动登录。

常见 reason code：

- `unauthenticated`：前端没有找到可用会话。
- `session_refresh_failed`：Cookie 或会话刷新失败。
- `authentication_failed`：API 认证重试失败。
- `insufficient_permissions`：当前用户没有匹配的 Kite RBAC 权限。
- `token_exchange_failed`、`user_info_failed`、`jwt_generation_failed`、`callback_failed`、`callback_error`：OAuth 回调或会话创建失败。
- `user_disabled`：Kite 用户存在，但已被禁用。

建议检查：

1. 查看后端日志里的数据库、迁移、JWT、OAuth、Sealos 认证或会话创建错误。
2. 检查 `DB_TYPE`、`DB_DSN` 和数据库账号权限。若 `DB_AUTO_CREATE=true`，MySQL/PostgreSQL 账号需要有创建 database 的权限。
3. 检查 `JWT_SECRET`、`KITE_ENCRYPT_KEY`、`SEALOS_AUTH_ENABLED`、`SEALOS_JWT_SECRET` 和 OAuth provider 回调配置。
4. 确认浏览器能收到并发送 `auth_token` Cookie。iframe/跨站部署需要检查 `AUTH_COOKIE_SAMESITE=none`、`AUTH_COOKIE_SECURE=true` 和 HTTPS。
5. 对于 `insufficient_permissions`，检查 Kite RBAC 角色分配和 OAuth 组/用户映射。

## 数据库排障

数据库启动故障通常会发生在完整 UI 可用之前。重点查找如下 panic：

- `failed to ensure database exists`
- `failed to connect database`
- `failed to migrate database`
- `database connection is nil`

SQLite hostPath 问题见 `docs/zh/faq.md`。生产持久化建议优先使用 MySQL 或 PostgreSQL。

## Kubernetes 连接排障

- 托管 Kubernetes 的 kubeconfig 如果使用 `aws`、`gcloud`、`kubelogin` 这类 `exec` 插件，应改用 Service Account token kubeconfig。参考 `docs/zh/config/managed-k8s-auth.md`。
- 如果访问资源时报权限错误，先检查 Kite RBAC，再检查服务账号或导入 kubeconfig 对应的 Kubernetes RBAC。
- 如果 Sealos 个人工作空间一直停留在加载状态，同时后端日志反复出现 `failed to wait for cache sync`，先检查 kubeconfig current-context namespace。非豁免命名空间作用域 kubeconfig 应走直接 Kubernetes API 读取，而不是 controller-runtime informer cache；后端集群构建失败会被限频，新的 Sealos 登录或集群配置更新会立即重试。
- 如果生产镜像没有 shell，不要强行 `kubectl exec` 进 Kite，改用临时 debug/client pod。

## AI 助手运维

- AI 助手 UI 在新安装中默认启用。可见设置入口只保留在 AI 聊天面板的配置按钮里；只有 Kite 管理员可以编辑设置页，且该页面只显示 AI Agent 设置。
- 聊天请求需要 OpenAI-compatible 或 Anthropic-compatible provider 配置和 API Key。缺少 API Key 时，运行时会把 AI 视为未启用。
- 只读工具仍使用当前认证用户、集群和命名空间作用域。
- 在非豁免的 Sealos 命名空间作用域工作空间中，AI 工具会把省略的 namespace 和 `_all` 解析为当前工作空间命名空间，拒绝普通集群级资源（如 Nodes/Namespaces），隐藏任意 Prometheus 查询，并只展示命名空间内的集群概览。
- 变更资源的工具需要同时满足 Kite RBAC，并经过显式继续/确认步骤。Pending session 会绑定同一用户和集群。
- AI 聊天里的 Helm 操作是结构化 tool call，底层复用 Kite 的 Helm SDK 集成，而不是在 Kite Pod 里直接执行 `helm` shell 命令。因此 AI Helm 工作流不要求镜像内置 Helm CLI。
- AI Helm install/upgrade 应先执行对应 dry-run 工具。dry-run 响应会汇总渲染出的资源供用户确认；真正的 install、upgrade、rollback、uninstall 工具随后进入与其他变更工具相同的确认流程。
- AI Helm 工具既需要目标命名空间上的 `helmreleases` RBAC，也会通过 `pkg/helmguard` 校验渲染出的 Kubernetes 资源权限。如果报权限错误，同时检查目标 namespace 的 `helmreleases` verb 和 dry-run 资源摘要里的实际资源权限。
- 如果 AI chat 返回 AI Agent configuration、disabled 或 provider 错误，检查通用设置记录、provider Base URL、API Key、模型名，以及当前用户编辑 `/settings` 时是否拥有 Kite 内置 `admin` 角色。

## Helm 运维

- Chart catalog 只读 API 面向认证用户开放在 `/api/v1/charts`。仓库创建/删除和 catalog 管理 API 仍在 `/api/v1/admin/charts`，只允许管理员使用；响应里不能暴露已存储的仓库凭据。
- 离线环境可以通过 `KITE_HELM_ARTIFACT_HUB_ENABLED=false` 或 `helmCatalog.artifactHub.enabled=false` 关闭 Artifact Hub。关闭后，后端 Artifact Hub 代理接口和前端 fallback 都不会再使用线上 Artifact Hub。
- Kite 通过 `helmCatalog.oci.base` / `KITE_HELM_OCI_REGISTRY_BASE` 扫描受控 OCI registry 前缀来发现离线 Helm Chart；只会暴露该前缀下的内容，不做无边界的全 registry 展示。
- registry 凭据和 TLS 配置只在服务端使用。Chart 只读 API 返回干净的 `oci://host/prefix/chart:version` URL，不包含凭据、查询参数或 fragment。Helm OCI tag 会把 SemVer build metadata 中的 `+` 编码为 `_`。
- 如果 OCI discovery 返回 `401 Unauthorized`，先确认用户名/密码 Secret 已注入，再检查 registry 的 `WWW-Authenticate` challenge。registry 要求 Bearer token exchange 时，直接发送 Basic 请求仍可能返回 401；Kite 同时兼容匿名、Basic 和 Bearer challenge。Bearer 失败时还要确认 token realm 可访问，并确保 `caFile` 或 `insecureSkipTLSVerify` 能覆盖它的 HTTPS 证书，包括 registry API 本身使用 `plainHTTP` 的组合场景。
- 离线 Chart artifact 和容器镜像可以共用同一个 registry host，但使用不同 repository 路径。Chart 放在专用前缀下，例如 `oci://registry.internal/kite-helm/<chart>:<version>`；容器镜像放在原始仓库路径下，例如 `registry.internal/bitnami/nginx:<tag>`。
- 管理员 UI 以 `.kiteapp.tar.gz` 离线应用包作为传递单位，不再把 Chart 和镜像拆成两个上传入口。一个包包含 `kite-bundle.json`、一个或多个 Helm chart 归档，以及这些 Chart 按离线镜像 values 渲染后需要的容器镜像归档。导入时会先校验包内容，推送所有必需镜像，再最后推送 Chart，避免 catalog 中出现缺镜像的 Chart。UI 会在离线包上传被服务端接受后启动后台导入任务，在弹窗外轮询任务状态，让用户可以继续操作；任务状态保存在内存里，Kite Pod 重启后可能丢失。导出会把选中的 OCI catalog Chart 和渲染出的 workload 镜像打包，方便导入到另一个已配置好的 Kite 集群。
- 底层后端上传能力仍保持分离，用于兼容和内部复用。Helm chart 包走 `POST /api/v1/admin/charts/oci/upload`，推送到 `helmCatalog.oci.base` 下；容器镜像归档走 `POST /api/v1/admin/images/upload`，推送到 `helmCatalog.imageUploads.registry` 加 `helmCatalog.imageUploads.repositoryPrefix` 下。离线应用包后台导入使用 `POST /api/v1/admin/charts/offline-bundles/import-jobs` 和 `GET /api/v1/admin/charts/offline-bundles/import-jobs/:id`，兼容保留同步导入接口 `POST /api/v1/admin/charts/offline-bundles/import`，导出接口是 `POST /api/v1/admin/charts/offline-bundles/export`。
- 最小 OCI discovery 示例：

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

如果 `helmCatalog.imageUploads.registry` 为空，Kite 会在已配置
`helmCatalog.offlineImages.registry` 时复用它作为镜像上传 registry。Chart OCI
registry 和镜像上传 registry 凭据不同时，请使用不同 Secret key。上传大小限制会
渲染为 `KITE_HELM_OCI_UPLOAD_MAX_BYTES` 和 `KITE_IMAGE_UPLOAD_MAX_BYTES`；
ingress 或网关 body-size limit 至少要高于服务端限制。离线应用包上传同样受镜像归档大小限制约束。

- 如果不通过 Kite 的离线应用包导出准备 Chart，安装离线 OCI Chart 前需先同步它渲染出的 workload 容器镜像：

```bash
scripts/mirror-helm-chart-images.sh \
  --chart oci://registry.internal/kite-helm/nginx \
  --version 25.0.12 \
  --registry registry.internal
```

先加 `--dry-run` 查看复制计划。脚本会渲染原始 Chart 和注入离线 registry 后
的目标 Chart，按 Kubernetes 资源和容器身份映射 source image 到 target image，
去重后用 `crane copy` 复制。目标渲染时也会注入
`global.security.allowInsecureImages=true`，避免
Bitnami 这类 Chart 因离线 registry 替换触发镜像校验失败。

- 当 `helmCatalog.offlineImages.enabled=true` 时，Kite 会对 OCI catalog 的
安装、升级、AI Helm 工具和定时自动升级应用同一套离线镜像默认值。仅在用户
未显式设置时注入 `global.imageRegistry` 和
`global.security.allowInsecureImages`，然后 dry-run release 并检查 Pod、
Deployment、StatefulSet、DaemonSet、ReplicaSet、ReplicationController、
Job、CronJob 中的容器镜像。若 `helmCatalog.offlineImages.enforce=true`，
任何仍指向 `helmCatalog.offlineImages.registry` 之外的 workload 镜像都会在
写入集群前阻止安装/升级。Kite 会在安装或显式切换 Chart 时把 Chart 来源写入
Helm chart metadata，所以后续沿用当前 Chart 的升级仍会继续应用 OCI 离线镜像
策略。

- Helm Release API 使用规范资源路径 `helmreleases`。旧的 `helmrelease` 路由仅保留兼容。
- Helm install、upgrade、rollback、uninstall 和 auto-upgrade 在写入前都会先渲染目标清单。新增资源需要 `create`，保留资源需要 `update`，被移除资源需要 `delete`。
- AI 助手复用与 HTTP Helm Release API 相同的 Helm SDK 和 rendered-manifest guard 路径。除非是独立 debug 会话明确需要，否则不要通过给 Kite 容器安装 shell 或 Helm CLI 来排查 AI Helm 失败。
- CRD、Namespace、ClusterRole、ClusterRoleBinding 等集群级渲染资源需要 Kite admin 角色，并且会在命名空间作用域的 Sealos 集群中被拒绝。
- 如果 upgrade 或 rollback 因渲染资源权限失败，查看 dry-run manifest diff，并补齐 Kite RBAC 中具体资源的 verb，或选择保持在用户命名空间作用域内的 chart/values。

## Sealos 打包

- Sealos 部署入口会从 `/root/.sealos/cloud/scripts/tools.sh` 加载平台 helper，读取全局 HTTP/TLS 设置，并把 `cloudDomain`、`cloudPort`、`httpPort`、`disableHttps`、`certSecretName`、`platform.tlsRejectUnauthorized` 和 `db.postgres.native.kubeblocksVersion` 传给 Helm。
- 应用覆盖 values 从 `/root/.sealos/cloud/values/apps/kite/*-values.yaml` 按排序顺序加载。若该目录没有 `*-values.yaml`，入口会先复制 chart 默认的 `kite-values.yaml`，再执行 Helm。
- Release workflow 会为 Sealos 镜像包生成 `.tar.gz.md5`，并附加到 GitHub artifact/release。当前设计不上传镜像包或校验文件到 OSS。

## 生产镜像说明

生产部署默认构建和发布 `linux/amd64` 镜像，除非明确要求 ARM。
