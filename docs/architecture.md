# Panda 项目结构联系图

> 本文件位于 PandaX 仓库 `docs/` 下，随代码版本管理。文中 `PandaX/`、`PandaUi/`、`zigbee-bridge/` 等指工作区中的同级目录。

说明各子项目、运行组件、PandaX 内部模块与数据存储之间的关系。所有端口与路径均以当前仓库代码/`config.yml` 为准。

---

## 1. 全局视图

```mermaid
flowchart LR
    subgraph Client["客户端"]
        B["浏览器<br/>admin/123456"]
        SC["组态大屏<br/>(独立项目)"]
        DEV["设备/网关程序<br/>device-sdk 或第三方"]
    end

    subgraph FE["PandaUi (Vue3) :7789"]
        UI["views / api / stores"]
    end

    subgraph BE["PandaX (Go) :7788"]
        API["REST API<br/>go-restful"]
        IOT["iothub<br/>gRPC:9001 / HTTP:9002 / TCP/UDP:9003"]
        RULE["规则链<br/>rule_engine"]
        WS["websocket<br/>GET /system/"]
    end

    subgraph INFRA["依赖服务"]
        MY[("MySQL :3311<br/>pandax_iot")]
        RD[("Redis :6379")]
        TD[("TDengine :6041<br/>iot")]
        MQ["EMQX :1883<br/>Dashboard :18083"]
    end

    BR["zigbee-bridge"]
    Z2M["Zigbee2MQTT + Mosquitto<br/>192.168.21.151"]
    ZG["Zigbee 传感器<br/>ZG-227Z / ZG-203Z"]

    B --> UI
    UI -->|"axios X-TOKEN<br/>/api → 7788"| API
    SC -.->|"websocket"| WS
    API --> MY
    API --> RD
    API --> TD
    API --> WS
    MQ -->|"exhook 全部 hook<br/>host.docker.internal:9001"| IOT
    IOT --> RULE
    RULE --> TD
    RULE --> MY
    IOT --> WS
    DEV -->|"MQTT v1/devices/me/*<br/>用户名=设备AuthToken"| MQ
    ZG --> Z2M --> BR -->|"v1/gateway/telemetry<br/>v1/gateway/connect"| MQ

    UI -.->|"设备详情/遥测查询"| API
```

ASCII 版（便于终端查看）：

```
浏览器 ──> PandaUi(7789) ──/api 代理──> PandaX REST(7788) ──> MySQL / Redis
                                              │
                                              ├──> TDengine(6041) 时序数据
                                              └──> websocket GET /system/ ──> 组态大屏

Zigbee 传感器 ──> Z2M ──> zigbee-bridge ──> EMQX(1883) ──exhook:9001──> iothub ──> 规则链 ──> TDengine
```

---

## 2. 子项目关系

```mermaid
flowchart TD
    PX["PandaX<br/>后端主服务"]
    PU["PandaUi<br/>管理前端"]
    SDK["device-sdk<br/>设备端 MQTT 库"]
    BR["zigbee-bridge<br/>协议转换"]
    SC["screen<br/>静态演示页"]

    PU -->|"HTTP + X-TOKEN"| PX
    BR -->|"MQTT 网关主题"| PX
    SDK -.->|"MQTT 直连主题"| PX
    PU -.->|"VITE_SCREEN_URL"| SC
    PU -.->|"VITE_RULE_URL / VITE_REPORT_URL"| PX
```

| 子项目 | 角色 | 是否可独立启动 | 被谁使用 |
|---|---|---|---|
| `PandaX` | 服务端：API + 设备接入 + 规则引擎 | 是（`pandax.exe`） | PandaUi、设备、桥接 |
| `PandaUi` | 管理界面 | 是（`npm run dev`） | 人 |
| `device-sdk` | 设备端库：封装主题/报文/重连 | 否（只能被 import） | 用户自研设备程序 |
| `zigbee-bridge` | 把 Z2M 报文转成 PandaX 网关主题 | 是 | 无（独立运行） |
| `screen` | 大屏静态占位页 | 是（静态服务器） | PandaUi 菜单跳转 |

关键点：**PandaX 不依赖 device-sdk**（`go.mod` 中无此模块），二者是同一套 MQTT 协议的两端实现。

---

## 3. PandaX 启动与请求管线

```mermaid
sequenceDiagram
    participant CLI as pandax.exe
    participant Pre as PreRun
    participant Run as Run
    participant MW as 拦截器链
    participant API as 业务 handler
    participant DB as MySQL/Redis/TDengine

    CLI->>Pre: 读取 config.yml
    Pre->>DB: MySQL 连接 → Redis 连接 → TDengine 连接
    Pre->>Pre: InitTable(可选 AutoMigrate) + go InitEvents()
    CLI->>Run: 执行
    Run->>MW: UseBefore(FlowBeforeHandler)
    Run->>MW: UseAfter(LogHandler → OperationHandler → FlowAfterHandler)
    Run->>API: InitRouter() 注册所有 webservice
    API-->>CLI: app.Start() 监听 7788
    CLI->>CLI: go iothub.InitIothub()
```

单个请求的处理顺序：

```
go-restful Filter: EscapeHTML → Cors → OPTIONSFilter → Rate
        │
        ▼
FlowBeforeHandler ── 记录「请求接入」→ 调用 PermissionHandler(JWT + casbin)
        │                └─ 记录「鉴权校验」OK/FAIL
        ▼
业务 handler (ReqCtx.Handle)  ── panic 收敛为 rc.Err，写统一响应
        │
        ▼
LogHandler → OperationHandler(落 log_opers + 渲染流程图) → FlowAfterHandler(兜底打印/清理)
```

---

## 4. iothub 数据流

```mermaid
flowchart LR
    subgraph Access["接入层 iothub/server"]
        EMQX["emqxserver :9001<br/>exhook 全量 hook"]
        HTTP["httpserver :9002<br/>HTTP 上报"]
        TCP["tcpserver :9003"]
        UDP["udpserver :9003"]
    end

    HK["hook_message_work<br/>HookService<br/>MessageCh → MessageWork"]
    AUTH["netbase<br/>Auth / SubAuth / GetUserName"]
    CACHE[("Redis<br/>设备 AuthToken 缓存")]
    RE["rule_engine<br/>按 productId 取实例"]
    SH["shadow<br/>在线状态/设备影子"]
    TD[("TDengine<br/>iot.<device>_telemetry")]
    WS["websocket<br/>Wsp[screenId]"]

    EMQX --> HK
    HTTP --> HK
    TCP --> HK
    UDP --> HK
    HK --> AUTH
    AUTH --> CACHE
    AUTH -->|"未命中则查 MySQL devices.token"| CACHE
    HK -->|"Telemetry/Attributes/Row/Event"| RE
    HK -->|"Connect / DisConnect"| SH
    HK -->|"Telemetry"| WS
    RE -->|"SaveTimeSeriesNode"| TD
```

要点：

- EMQX 的 hook 回调地址由 `deploy/emqx-exhook.conf` 指定为 `http://host.docker.internal:9001`，**容器重建后需重新 `emqx ctl conf load`**。
- 认证在 `OnClientAuthenticate`：用户名必须命中 `devices.token`（或 `config.yml` 的 `mqtt.username`），否则 `Not authorized`。
- 消息进入 `HookService.MessageCh`，由 `MessageWork` 单协程消费（并发受 `queue` 配置限制）。
- 规则链实例按 **productId** 缓存；`InitEvents` 监听 `ProductChainRuleEvent`，规则链变更后重建实例并刷新所有绑定产品。
- 遥测会同时推给 websocket 大屏（`{"MESSAGETYPE":"01","MESSAGECONTENT":{"twinId":设备ID,"attrs":{...}}}`），PandaUi 本身不使用该通道。

规则链节点（`pkg/rule_engine/nodes`）分四类：

| 类别 | 节点 |
|---|---|
| 输入 | `InputNode` |
| 过滤/分支 | `MessageTypeSwitchNode`、`SwitchNode`、`ScriptNode`、`DeviceTypeSwitchNode` |
| 动作 | `SaveTimeSeriesNode`、`SaveAttributesNode`、`CreateAlarmNode`、`ClearAlarmNode`、`DelayNode`、`LogNode`、`RpcRequestToDeviceNode` |
| 外部/转换 | `MqttNode`、`KafkaNode`、`NatsNode`、`RestApiNode`、`RuleChainNode`、`DingNode`、`EmailNode`、`SmsNode`、`WechatNode`、`ScriptTransformNode`、`RenameKeyNode`、`DeleteKeyNode` |

---

## 5. PandaUi 内部结构

```mermaid
flowchart TD
    ROUTER["router/index.ts<br/>hash 模式 + 后端菜单动态注册"]
    STORE["stores/<br/>themeConfig / userInfos / routesList / tagsViewRoutes"]
    VIEW["views/<模块><br/>列表页 + component 弹窗"]
    API["api/<模块>/*.ts<br/>axios 封装"]
    REQ["utils/request.ts<br/>baseURL=VITE_API_URL，自动带 X-TOKEN"]
    BE["PandaX :7788"]

    ROUTER --> VIEW
    VIEW -->|"proxy.getDicts / proxy"| STORE
    VIEW --> API --> REQ --> BE
    BE -->|"401/4001 清登录态"| REQ
```

- 菜单与按钮权限来自后端（`/system/user/auth`），前端不写死路由树。
- 字典统一走 `proxy.getDicts(type)` + `proxy.selectDictLabel()`。
- dev 环境下 vite 把 `/api` 代理到 `127.0.0.1:7788`，同时 `VITE_API_URL` 直接指向 7788（两套都可用）。

---

## 6. 数据模型关系

```mermaid
erDiagram
    sys_users ||--|| sys_roles : role_id
    sys_users }o--|| sys_organizations : organization_id
    sys_roles ||--o{ sys_role_menus : role_id
    sys_menus ||--o{ sys_role_menus : menu_id
    sys_roles ||--o{ casbin_rule : "v0=roleKey"
    sys_apis }o--o{ casbin_rule : "v1=path, v2=method"

    products ||--o{ product_templates : pid
    products ||--o{ devices : pid
    device_groups ||--o{ devices : gid
    products }o--|| rule_chain : rule_chain_id
    devices ||--o{ device_alarms : "deviceId"
    devices ||--o{ device_cmd_log : "deviceId"

    log_opers }o--|| sys_users : oper_name
```

TDengine（库 `iot`）：

| 表 | 说明 |
|---|---|
| `<productId>_telemetry` / `<productId>_attributes` | 产品级超级表（建产品时创建） |
| `<deviceName>_telemetry` / `<deviceName>_attributes` | 设备子表（建设备时创建，名称小写） |
| `events` | 设备上下线、事件上报（`CreateEventTable`） |
| `logs` / `rule_debug_*` | 规则链调试与运行日志 |

MySQL、`log_opers` 的 `trace_id` / `flow` / `flow_data` / `error_msg` 为链路图功能新增字段，改结构时务必同步 `PandaX/resource/pandax_iot.sql`。

---

## 7. 设备接入实例（Zigbee 温湿度）

```mermaid
sequenceDiagram
    participant S as ZG-227Z
    participant Z as Zigbee2MQTT
    participant M as Mosquitto
    participant B as zigbee-bridge
    participant E as EMQX
    participant P as PandaX iothub
    participant T as TDengine

    S->>Z: Zigbee 上报 (~30s)
    Z->>M: zigbee2mqtt/0xa4c138a8230ea245
    M->>B: 订阅 zigbee2mqtt/+
    B->>E: v1/gateway/telemetry (用户名=zigbeegw 的 AuthToken)
    B->>E: v1/gateway/connect
    E->>P: exhook message.publish (9001)
    P->>P: SubAuth(sensor01) → 规则链
    P->>T: INSERT iot.sensor01_telemetry
    Note over P,T: 设备详情「设备状态 → 遥测」读取该表最新一行
```

---

## 8. 端口与依赖

| 组件 | 端口 | 配置位置 |
|---|---|---|
| PandaX REST | 7788 | `config.yml: server.port` |
| EMQX exhook gRPC | 9001 | `server.grpc-port` |
| iothub HTTP hook | 9002 | `server.http-port` |
| iothub TCP / UDP | 9003 | `server.tcp-port`（UDP 复用该端口） |
| PandaUi dev | 7789 | `PandaUi/.env: VITE_PORT` |
| EMQX MQTT / Dashboard | 1883 / 18083 | 容器 |
| MySQL | 3311 → 3306 | 容器映射（本机 3306 被占用） |
| Redis | 6379 | 容器 |
| TDengine REST | 6041 | 容器 |

---

## 9. 目录速查

```
Panda/                     （工作区根，非 git 仓库）
├── PandaX/                后端：apps(业务四层) / pkg(通用能力) / iothub(接入) / resource(SQL)
│   └── docs/              本文档与工作区总览（workspace.md）
├── PandaUi/               前端：src/{api,views,router,stores,utils}
├── device-sdk/            设备端 MQTT 库（client / spec / samples）
├── zigbee-bridge/         Z2M → PandaX 转换器
└── screen/                大屏静态演示页
```

各子项目的详细约定见其仓库内的 `AGENTS.md`；工作区总览见 [workspace.md](workspace.md)。
