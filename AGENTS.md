# PandaX 后端

Go 1.20 物联网平台后端：REST API（go-restful）+ iothub 设备接入（EMQX exhook / TCP / UDP / HTTP）。

## 构建与运行

```powershell
go build -o pandax.exe .          # 或 go run main.go
.\pandax.exe                      # 默认读 ./config.yml，可用 --config 或 PANDA_CONFIG 覆盖
.\pandax.exe --config config.local.yml   # 本机差异（如 MySQL 端口 3311）放在 config.local.yml，该文件已 gitignore

go test ./pkg/flowlog/ ./pkg/tool/          # 目前可用的测试
go vet ./pkg/middleware/ ./pkg/flowlog/
```

既有失败（**与你的改动无关，不要顺手重写**）：
- `pkg/rule_engine/instance_test.go:29` 调用了不存在的 `RuleChainInstance.StartRuleChain`，导致 `go test ./pkg/...` 构建失败；
- `pkg/tdengine/tdengine_log.go:23` 的 `fmt.Sprintf` 无格式符却传参，`go vet` 报错并连带 `go test` 失败。

服务端口来自 `config.yml`：`server.port`(7788)、`grpc-port`(9001)、`http-port`(9002)、`tcp-port`(9003)。

## 目录结构

详见 [docs/architecture.md](docs/architecture.md)（模块关系图与端口/数据模型速查）、[docs/workspace.md](docs/workspace.md)（工作区总览与 Git 协作方式）。

```
main.go                      # cobra 入口：PreRun 连库/缓存/时序库，Run 注册拦截器 + 路由 + iothub
apps/<模块>/{api,entity,router,services}
                             # system / device / log / job / rule / develop
pkg/initialize/              # router.go 注册所有 webservice；table.go 按 isInitTable 自动建表
pkg/middleware/              # flow(链路图) permission(鉴权) oper(操作日志) log cors rate escape_html
pkg/flowlog/                 # ASCII 流程图渲染（CJK 宽度对齐），有单测
pkg/tdengine/                # 时序库：超级表/设备表/事件/日志，走 REST(6041) 驱动
pkg/rule_engine/             # 规则链实例与节点（Input/MessageTypeSwitch/SaveTimeSeries...）
pkg/shadow cache events websocket device_rpc transport tool global config
iothub/                      # EMQX hook 服务端、TCP/UDP/HTTP 接入、hook_message_work
resource/                    # pandax_iot.sql、rbac_model.conf、excel 模板
deploy/                      # k8s / docker-compose / emqx-exhook.conf
```

## 新增业务模块的固定套路

1. `apps/<mod>/entity`：结构体嵌入 `model.BaseAuthModel`（带 owner/org）或 `model.BaseModel`，加 gorm tag
2. `apps/<mod>/services`：`table` 常量 + `global.Db.Table(m.table)`，接口定义为 `XxxModel`
3. `apps/<mod>/api`：`restfulx.NewReqCtx(request, response).WithLog("中文描述").Handle(handler)`，用 `rc.ResData` 回数据，`biz.ErrIsNil` 抛业务错误
4. `apps/<mod>/router`：`ws.Path("/xxx").Produces(restful.MIME_JSON)`，每个路由挂 `Doc(...)`、参数说明
5. `pkg/initialize/router.go` 注册：`xxxRouter.InitXxxRouter(container)`
6. 权限数据：`sys_apis`、`sys_menus`、`casbin_rule`，否则前端按钮/接口都会 403

## 请求处理管线

- go-restful Filter（`initialize/router.go`）：`EscapeHTML` → `Cors` → `OPTIONSFilter` → `Rate`
- 前置拦截器（`main.go`，仅一个）：`middleware.FlowBeforeHandler`，内部先记录「请求接入」，再调用 `PermissionHandler`（JWT 解析 + casbin `Enforce(roleKey, path, method)`）
- 业务 handler（`ReqCtx.Handle`：panic → `rc.Err` → 统一错误响应）
- 后置拦截器（按注册顺序）：`LogHandler` → `OperationHandler` → `FlowAfterHandler`
- 链路图状态用 `middleware.flowStore`（`sync.Map`，key 为 `*restfulx.ReqCtx`）传递，`FlowAfterHandler` 负责删除

公开接口必须显式声明：`NewReqCtx(...).WithNeedToken(false).WithNeedCasbin(false)`（如登录、验证码）。

## 操作日志与链路图

- 表 `log_opers`：`status`（0 成功 / 1 失败，由 `rc.Err` 决定）、`error_msg`、`trace_id`、`flow`（ASCII 原文）、`flow_data`（步骤 JSON，前端渲染用）
- 只记录非 GET 且需要 token 的请求；GET 成功不打印流程图，失败会打印
- 请求体最多采集 4KB，`multipart/form-data` 不采集；`password/token/secret` 等字段自动脱敏
- 流程图格式：`[STEP n/total] ──▶ 名称` + `url/from/body/headers/cost/status/error`，末尾 `FLOW DONE: SUCCESS|FAILED`

## 已知坑

- **TDengine 不支持 `?` 绑定表名/字段名**：拼 SQL 前用 `tdIdentifier()` 校验标识符；`GetOne/GetAll` 返回列值是指针，用 `derefValue()` 解引用。
- **Casbin 初始数据不全**：`resource/pandax_iot.sql` 里 `manage` 角色只有 GET 策略，而 `admin` 账号就属于 `manage` → 出现「没有API接口访问权限」。修复：以 `admin` 角色补齐策略，或在「角色管理 → 修改」里重新保存 API 权限触发 `UpdateCasbin`。
- **改表结构要同步 `resource/pandax_iot.sql`**，同时给实体加 gorm tag；`isInitTable=true` 只做 AutoMigrate，不会回填数据。
- **设备认证**：MQTT 用户名必须是 `devices.token`（网关/直连设备）或 `config.yml` 的 `mqtt.username`；token 同时缓存在 Redis（`cache.SetDeviceEtoken`）。
- **iothub 主题**（上行）：`v1/devices/me/telemetry`、`/attributes`、`/row`、`/rpc/request|response`、`v1/gateway/telemetry|attributes|connect`。
- 本机 3306 被占用，`config.yml` 的 MySQL 用 `127.0.0.1:3311`；改端口/账号后必须重启进程。
- 日志写到 stderr（`InitLog` 未配置 file 时），常用 `.\pandax.exe 2>pandax-run.err` 收集。

## 验证

```powershell
curl.exe -s -o NUL -w "%{http_code}" http://127.0.0.1:7788/system/user/getCaptcha
docker exec pandax-emqx sh -c "emqx ctl conf show exhook"      # 必须指向 host.docker.internal:9001
```
