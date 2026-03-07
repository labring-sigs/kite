# Kite - 现代化的 Kubernetes Dashboard

<div align="center">

<img src="./docs/assets/logo.svg" alt="Kite Logo" width="128" height="128">

_一个现代化、直观的 Kubernetes Dashboard_

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org)
[![React](https://img.shields.io/badge/React-19+-61DAFB?style=flat&logo=react)](https://reactjs.org)
[![TypeScript](https://img.shields.io/badge/TypeScript-5+-3178C6?style=flat&logo=typescript)](https://www.typescriptlang.org)
[![License](https://img.shields.io/badge/License-Apache-green.svg)](LICENSE)

[**在线 Demo**](https://kite-demo.zzde.me) | [**文档**](https://kite.zzde.me)
<br>
[English](./README.md) | **中文**

</div>

Kite 是一个轻量级、现代化的 Kubernetes Dashboard，为管理和监控您的 Kubernetes 集群提供了一个直观的界面。它提供实时指标、全面的资源管理、多集群支持和优美的用户体验。

> [!WARNING]
> 本项目正在快速迭代开发中，使用方式和 API 都有可能变化。

![Dashboard Overview](docs/screenshots/overview.png)
_全面的集群概览，包含实时指标和资源统计_

## ✨ 功能特性

### 🎯 **现代化的用户体验**

- 🌓 **多主题支持** - 暗色/亮色/彩色主题，并能自动适应系统偏好
- 🔍 **高级搜索** - 支持跨所有资源的全局搜索
- 🌐 **国际化支持** - 支持英文和中文语言
- 📱 **响应式设计** - 针对桌面、平板和移动设备优化

### 🏘️ **多集群管理**

- 🔄 **无缝集群切换** - 可在多个 Kubernetes 集群之间切换
- 📊 **分集群监控** - 每个集群可独立配置 Prometheus
- 🔐 **集群访问控制** - 集群访问管理的细粒度权限控制

### 🔍 **全面的资源管理**

- 📋 **全资源覆盖** - 支持 Pods, Deployments, Services, ConfigMaps, Secrets, PVs, PVCs, Nodes 等
- 📄 **实时 YAML 编辑** - 内置 Monaco 编辑器，支持语法高亮和校验
- 📊 **详细的资源视图** - 提供容器、卷、事件和状况等深入信息
- 🔗 **资源关系可视化** - 可视化相关资源之间的连接（例如，Deployment → Pods）
- ⚙️ **资源操作** - 直接从 UI 创建、更新、删除、扩缩容和重启资源
- 🔄 **自定义资源** - 完全支持 CRD (Custom Resource Definitions)
- 🏷️ **镜像标签快速选择器** - 基于 Docker 和容器镜像仓库 API，轻松选择和更改容器镜像标签
- 🎨 **自定义侧边栏** - 自定义侧边栏的可见性和顺序，并添加 CRD 以方便快速访问
- 🔌 **Kube Proxy** - 通过 Kite 直接访问 Pods 或 Services，无需 `kubectl port-forward`

### 📈 **监控与可观测性**

- 📊 **实时指标** - 由 Prometheus 驱动的 CPU、内存、磁盘 I/O 和网络使用情况图表
- 📋 **集群概览** - 全面的集群健康状况和资源统计仪表板
- 📝 **实时日志** - 实时流式传输 Pod 日志，支持过滤和搜索
- 💻 **网页终端** - 直接在浏览器中进入 Pod/Node 执行命令
- 📈 **节点监控** - 详细的节点级别性能指标和利用率
- 📊 **Pod 监控** - 单个 Pod 资源使用情况和性能跟踪

### 🔐 **安全**

- 🛡️ **OAuth 集成** - 支持在 UI 管理 OAuth
- 🔒 **基于角色的访问控制** - 支持在 UI 管理用户的权限
- 👥 **用户管理** - 完整的用户管理和角色分配
- 🔐 **权限粒度** - 资源级别的精确访问控制权限

---

## 🚀 快速开始

有关详细说明，请参阅[文档](https://kite.zzde.me/guide/installation.html)。

### Docker

要使用 Docker 运行 Kite，您可以使用预构建的镜像：

```bash
docker run --rm -p 8080:8080 ghcr.io/zxh326/kite:latest
```

### 在 Kubernetes 中部署

#### 使用 Helm (推荐)

1.  **添加 Helm 仓库**

    ```bash
    helm repo add kite https://zxh326.github.io/kite
    helm repo update
    ```

2.  **使用默认值安装**

    ```bash
    helm install kite kite/kite -n kite-system
    ```

#### 使用 kubectl

1.  **应用部署清单**

    ```bash
    kubectl apply -f deploy/install-legacy.yaml
    # 或在线安装
    kubectl apply -f https://raw.githubusercontent.com/zxh326/kite/refs/heads/main/deploy/install-legacy.yaml
    ```

2.  **通过端口转发访问**

    ```bash
    kubectl port-forward -n kite-system svc/kite 8080:8080
    ```

### 从源码构建

#### 📋 准备工作

1.  **克隆仓库**

    ```bash
    git clone https://github.com/labring-sigs/kite.git
    cd kite
    ```

2.  **构建项目**

    ```bash
    make deps
    make build
    ```

3.  **运行服务**

    ```bash
    make run
    ```

---

## 🔍 问题排查

有关问题排查，请参阅[文档](https://kite.zzde.me)。

## 💖 支持本项目

如果您觉得 Kite 对您有帮助，请考虑支持本项目的开发！您的捐赠将帮助我们维护和改进这个项目。

### 捐赠方式

<table>
  <tr>
    <td align="center">
      <b>支付宝</b><br>
      <img src="./docs/donate/alipay.jpeg" alt="支付宝二维码" width="200">
    </td>
    <td align="center">
      <b>微信支付</b><br>
      <img src="./docs/donate/wechat.jpeg" alt="微信支付二维码" width="200">
    </td>
    <td align="center">
      <b>PayPal</b><br>
      <a href="https://www.paypal.me/zxh326">
        <img src="https://www.paypalobjects.com/webstatic/mktg/logo/pp_cc_mark_111x69.jpg" alt="PayPal" width="150">
      </a>
    </td>
  </tr>
</table>

感谢您的支持！❤️

## 🤝 贡献

我们欢迎贡献！请参阅我们的[贡献指南](https://kite.zzde.me/zh/faq.html#%E6%88%91%E5%9C%A8%E5%93%AA%E9%87%8C%E5%8F%AF%E4%BB%A5%E8%8E%B7%E5%BE%97%E5%B8%AE%E5%8A%A9)了解如何参与。

## 📄 许可证

本项目采用 Apache License 2.0 许可证 - 详见 [LICENSE](LICENSE) 文件。
