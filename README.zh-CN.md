# 喜乐闲鱼助手

> 一个把「找品 → 上架 → 客服 → 发货 → 售后」串成一条自动流水线的闲鱼运营工作台。Go + React 单二进制交付，Windows / macOS 提供免环境一键安装包。

## 加群交流

<img src="docs/images/wechat-group-qr.png" alt="微信交流群二维码" width="200">

> **加群请说明来意**：申请时请备注「喜乐闲鱼助手 + 你的使用场景」，例如「喜乐闲鱼助手 - 想了解商机挖掘」或「喜乐闲鱼助手 - 已购买，咨询安装」。不写来意的不予通过，避免广告引流。
>
> 群里可以交流：安装与使用问题、功能建议、闲鱼运营经验。**软件使用问题优先在群里问，回复更快。**

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)](frontend/package.json)
[![SQLite / MySQL / PostgreSQL](https://img.shields.io/badge/SQLite%20%7C%20MySQL%20%7C%20PostgreSQL-4479A1)](internal/db/migrations/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue)](LICENSE)
[![Release](https://img.shields.io/badge/Release-v2026.11.26-orange)](https://github.com/clearlove2008/xile-xianyu-helper/releases/tag/v2026.11.26)

[English README](README.md) · [功能特性](#功能特性) · [快速开始](#快速开始) · [界面导览](#界面导览) · [打包安装](#打包安装) · [配置说明](#配置说明) · [开发指南](#开发指南) · [开源协议与归属](#开源协议与归属)

> [!IMPORTANT]
> 本项目仅用于个人技术学习研究，**未获得闲鱼 / 阿里巴巴或任何网盘厂商的官方授权**。请自行评估并遵守相关平台的服务条款与当地法律法规。作者不对使用本软件产生的任何后果负责。

---

## 它解决什么问题

手工做闲鱼，最耗时的从来不是接单，而是这些重复动作：翻遍全网找能卖的品、逐个填写发布表单、盯着库存售罄再手动补上架、回复重复的咨询、转存网盘资源再生成自己的分享链接。

喜乐闲鱼助手把这些动作收进一个本地运行的工作台：

- **选品**：聚合 75 个搜索插件 + Telegram 频道 + 学霸盘，一次拿到上百条候选，自带整词过滤剔除无关结果。
- **上架**：商机详情多图预览、勾选账号批量铺货、自动处理多库存限制与发货地兜底。
- **运营**：售罄自动补货（默认 2 分钟巡检）、多账号自动任务、AI 回复、订单自动发货。
- **资源**：夸克 / 百度网盘扫码登录、Cookie 登录、一键转存并生成自己的分享链接，账号独立管理。

所有数据留在你自己的机器上（SQLite 默认），不经过第三方中转。

---

## 功能特性

### 1. 商机挖掘与商品工作流（本次发行版重点）

从「别人在卖什么」到「我已经上架了」的完整链路：

- **卖家商品搜索**：按关键词检索闲鱼在售商品，抓取标题、价格、图片详情，支持多图（已修复商机只能取到单图的问题）。
- **商机详情**：详情接口返回完整图片列表，前端支持多图预览，挑品不用再来回跳页面。
- **工作流任务**：把「搜索 → 筛选 → 一键上架」编排成可复用的工作流，任务状态持久化到 `00064_workflow_tasks.sql` 迁移。
- **一键上架**：从商机结果直接勾选商品、指定账号批量发布，走与手动发布相同的 MTOP 通道。

### 2. 全网聚合搜索

多源搜索能力**内嵌编译进同一个二进制**，不需要另起服务、不需要统一登录：

- **75 个搜索插件**：覆盖常见网盘搜索站、影视、软件、学习资料等频道，插件自注册，统一结果模型。
- **Telegram 频道搜索**：支持多频道配置，整词命中过滤（搜「八仙」不会再带出一堆无关资源），标题归属正确。
- **学霸盘插件**：对 `xuebapan.com` 做 HTML 搜索，5 页并发抓取，单次最多返回 **100 条**结果；附带详情页解析、提取码识别与促销关键词过滤。
- 搜索结果在「网盘搜索」页面按来源分类展示，可单独启停频道与插件。

### 3. 网盘转存与分享（夸克 + 百度）

独立的网盘账号管理，与闲鱼账号体系隔离：

- **登录方式**：二维码扫码登录（自动截取二维码、像素校验避免白图）、Cookie 登录两种；带真实账号态校验，游客态不会再被误判为已登录。
- **转存链路**：粘贴他人分享链接 → 解析提取码 → 转存到自己账号 → 生成你自己的新分享链接，全程自动。
- **会话稳定**：扫码过程的请求上下文不会因弹窗取消而误中断。
- 相关接口见 `internal/netdisk/` 与 `internal/server/netdisk_handlers.go`，数据落在 `00065_netdisk_accounts.sql` 迁移。

### 4. 发布、库存与售罄补货

- **多库存发布**：平台接受多件时按目标数量发布；平台明确拒绝多库存（`multi_inventory_not_allowed`）时自动降级为 1 件，随后补库存到目标值，账号级记住限制，不反复踩坑。
- **售罄自动补货**：独立巡检器默认每 **2 分钟**扫描一次售罄商品并重新上架；周期可通过 `reshelf_interval_minutes` 设置调整，失败有 30 分钟冷却，避免刷接口。
- **发货地兜底**：遇到 `FAIL_BIZ_ITEM_EDIT_INVALID_MAP_LOCATION`（虚拟商品未提交发货地）时，用高德附近 POI 映射出的默认发货地补齐，不再直接发布失败。
- **AI 出图发布**：支持多账号勾选，一次给多个账号生成并发布。

### 5. 商品图片批量替换

面向已经上架的商品，只替换图片、不改其他字段：

- `cmd/imgreplace` 工具支持 `dry_run`、`account_id`、`force_quantity`、`drop_fields` 与逐项 `items` 计划。
- 提交体只改 `imageInfoDOList` 中指定下标的图片信息（URL / 宽 / 高），保留原商品其他数据。
- 已知平台限制：部分商品在 PC 端返回 `FAIL_BIZ_PC_NOT_SUPPORT_PUBLISH_OR_EDIT`，这类商品需要在闲鱼 App 端手动换图；工具不会破坏线上原图。

### 6. 账号、消息与自动化

- **多账号管理**：账号分组、备注、登录态刷新、自动任务开关、暂停时长、运行状态一屏可见。
- **在线聊天**：WebSocket 实时消息、快捷回复、图片发送、会话内查看关联商品。
- **AI 回复**：可配置模型与回复策略，支持按订单上下文生成回复、价格调整建议。
- **自动任务**：自动发货、自动确认收货、议价处理、价格调整、凭证核销等动作，带审计事件记录（`00058_automation_audit_events.sql`）与失败熔断。
- **兜底机制**：付款系统消息丢失时按订单状态补一次自动发货并续跑未完成的待发货流程，同一订单 10 分钟内不重复兜底。

### 7. 素材库、规则与统计

- **素材库**：图片资源集中管理，支持从已上架商品引用图片（`00057` / `00059` 迁移）。
- **规则与模板**：默认回复、发货模板、商品问候语、卡片商品模板。
- **销售统计**：按时间范围的订单与销售额分析、仪表盘概览。

### 8. 免环境安装包

- **Windows 10+**：Inno Setup 打包，含内置 Playwright runtime 与 Chromium，双击安装即用，后台服务由 `service-control.ps1` 托管。
- **macOS 通用版**：交叉编译 amd64 + arm64 双架构二进制，pkg 安装到 `/Applications/喜乐闲鱼助手`，随包携带 Node 与 Chromium runtime。
- **Linux / Docker**：提供 install/uninstall 脚本、systemd unit 与多份 compose 文件（SQLite / PostgreSQL）。

---

## 界面导览

| 菜单 | 能做什么 |
| --- | --- |
| 仪表盘 | 账号状态、订单与销售概览、自动化告警 |
| 账号管理 | 多账号登录态、分组备注、自动任务设置 |
| 商机挖掘 | 商品搜索、多图详情、筛选后一键上架 |
| 商品工作流 | 编排搜索到上架的任务链，查看执行记录 |
| Search 搜索 | 75 个插件 + TG 频道 + 学霸盘聚合搜索 |
| 网盘转存 | 夸克/百度账号、扫码登录、转存并生成分享链接 |
| 在线聊天 | 实时会话、快捷回复、AI 回复 |
| 订单管理 | 订单同步、手动发货、缺单补录 |
| 素材库 | 图片与资源文件管理 |
| 自动化规则 | 默认回复、发货模板、AI 策略 |
| 销售统计 | 订单与销售额分析 |
| 系统设置 | 端口、数据库、通知、日志 |

服务默认监听 **`:59188`**，浏览器打开 `http://127.0.0.1:59188` 即可进入。

---

## 快速开始

### 方式一：安装包（推荐）

1. 到 [Releases](https://github.com/clearlove2008/xile-xianyu-helper/releases) 下载对应系统的安装包。
2. Windows：双击 `喜乐闲鱼助手-安装包-v2026.11.26.exe`，一路下一步；首次启动会初始化本地数据库。
3. macOS：双击 pkg 安装；由于未签名，首次打开需右键 → 打开，或在「系统设置 → 隐私与安全性」中放行。
4. 浏览器访问 `http://127.0.0.1:59188`，设置管理员密码后进入。

### 方式二：源码运行

环境要求：Go 1.26+、Node 20+。

```bash
git clone https://github.com/clearlove2008/xile-xianyu-helper.git
cd xile-xianyu-helper

# 前端
cd frontend && npm ci && npm run build && cd ..

# 后端（默认 SQLite，数据存到 ./data）
go run ./cmd/server

# 浏览器打开 http://127.0.0.1:59188
```

想用 PostgreSQL / MySQL，参考 `compose.yml` 与 `docs/wiki/配置参考.zh-CN.md`。

---

## 打包安装

源码自带打包脚本，可自行构建离线安装包：

| 平台 | 脚本 | 产物 |
| --- | --- | --- |
| Windows | `packaging/windows/installer-dist.iss` + `build-resources.ps1` | 含 runtime 的离线 exe 安装包 |
| macOS | `packaging/macos/build-xile-cross.py` | 通用版 pkg（amd64 + arm64） |
| Linux | `packaging/linux/install.sh` | systemd 服务安装 |

交叉编译与 runtime 准备细节见 `packaging/macos/README` 相关脚本注释。

---

## 配置说明

复制 `.env.example` 为 `.env` 后填写。**不要把填写后的 `.env` 提交到 Git**（已在 `.gitignore` 中排除）。

常用项：

| 变量 | 说明 |
| --- | --- |
| `XIANYU_HTTP_PORT` | 服务端口，默认 59188 |
| `XIANYU_BIND_ADDRESS` | 监听地址 |
| `DATABASE_URL` | 数据库连接串，默认走本地 SQLite |
| `XIANYU_DATA_KEY` | 本地数据加密密钥 |
| `XIANYU_ADMIN_PASSWORD` | 首次启动创建的管理员密码 |
| `BROWSER_HEADLESS` | 浏览器是否无头运行 |
| `LOG_LEVEL` / `LOG_FORMAT` | 日志级别与格式 |
| `CHANNELS` | Search Telegram 频道列表（逗号分隔） |

面板内的运行参数（如 `reshelf_interval_minutes`）在系统设置中可直接修改，无需重启。

---

## 开发指南

```bash
# Go 测试
go test ./...

# 前端测试
cd frontend && npm test

# 代码检查
golangci-lint run
```

仓库结构：

- `cmd/server` / `cmd/tray` — 服务与托盘入口
- `internal/xianyu/mtop` — 闲鱼 MTOP 接口（发布、搜索、订单、图片替换等）
- `internal/search` — 聚合搜索与 75 个插件
- `internal/netdisk` — 夸克 / 百度网盘客户端
- `internal/automation` / `internal/workflow` — 自动任务与工作流引擎
- `internal/db/migrations` — 三套数据库的迁移脚本
- `frontend/app/features` — 按业务拆分的 React 功能模块
- `packaging` — 各平台安装包脚本

---

## 开源协议与归属

本项目以 Apache License 2.0 发布，是在一个同为 Apache-2.0 授权的开源项目基础上二次开发的发行版。原始版权与许可证声明按协议要求保留在 [LICENSE](LICENSE) 与 [NOTICE](NOTICE)。

本发行版在其之上新增或改进了：商机挖掘与商品工作流、一键上架、聚合搜索内嵌（含 75 个插件与学霸盘）、TG 频道整词过滤、夸克/百度网盘转存、售罄自动补货、多库存降级、发货地兜底、多账号选号发布、商品图片批量替换，以及 Windows / macOS 离线安装包。

### 安全与隐私

- 本发行版源码已剔除全部运行时密钥、Cookie、数据库、日志与打包产物。
- 只提供 `.env.example` 模板；请勿把填写后的配置、账号 Cookie 或数据库文件提交到任何仓库。
- 所有账号凭据保存在你本机的数据目录中，本项目不会上传它们。
