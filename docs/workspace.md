# Panda 工作区总览

> 本文件随 PandaX 代码一起版本管理（仓库路径 `docs/workspace.md`）。文中 `PandaX/`、`PandaUi/` 等指工作区中的同级目录，不在本仓库内。

PandaX 物联网平台及其配套项目。各子项目细节见其仓库内的 `AGENTS.md`，后端细节见 [../AGENTS.md](../AGENTS.md)。

> 结构与模块关系图见 [architecture.md](architecture.md)（7 张 Mermaid 图 + 端口/数据模型速查）。

## 子项目索引

| 目录 | 说明 | 详见 |
|---|---|---|
| `PandaX/`（本仓库） | Go 后端主服务（REST API + iothub） | [AGENTS.md](../AGENTS.md) |
| `PandaUi/` | Vue3 + Vite 管理前端 | `PandaUi/AGENTS.md` |
| `device-sdk/` | 设备端 MQTT SDK（库，不单独启动） | `device-sdk/AGENTS.md` |
| `zigbee-bridge/` | Zigbee2MQTT → PandaX 协议转换 | `zigbee-bridge/AGENTS.md` |
| `screen/` | 组态大屏静态演示页 | `screen/AGENTS.md` |

## 启动顺序

```powershell
# 1. 依赖容器（MySQL / Redis / TDengine / EMQX）
docker start pandax-mysql pandax-redis pandax-tdengine pandax-emqx

# 2. 后端，7788
cd PandaX ; go build -o pandax.exe . ; .\pandax.exe

# 3. 前端，7789
cd PandaUi ; npm run dev
```

访问 `http://127.0.0.1:7789`，账号 `admin / 123456`（需输入验证码）。

## 端口与账号

| 服务 | 地址 | 账号 |
|---|---|---|
| 后端 API | 127.0.0.1:7788 | JWT（登录获取，请求头 `X-TOKEN`） |
| 前端 dev | 127.0.0.1:7789 | admin / 123456 |
| iothub gRPC（EMQX exhook） | 127.0.0.1:9001 | - |
| iothub TCP/UDP | 127.0.0.1:9003 | - |
| MySQL | 127.0.0.1:3311（容器 3306） | root / 123456，库 `pandax_iot` |
| Redis | 127.0.0.1:6379 | 123456 |
| TDengine | 127.0.0.1:6041（REST） | root / taosdata，库 `iot` |
| EMQX | 1883 / 18083 | Dashboard admin / public |

## 设备数据链路

```
Zigbee 传感器 --Zigbee--> Zigbee2MQTT(HA add-on) --MQTT--> Mosquitto
  --zigbee-bridge--> EMQX(1883, 用户名=设备AuthToken) --exhook--> PandaX iothub
  --> 规则链 --> TDengine(iot.<设备名>_telemetry)
```

## 跨项目约定

- 后端配置唯一来源 `PandaX/config.yml`；前端接口地址在 `PandaUi/.env.development`（`VITE_API_URL`）。
- 数据库结构变更必须同步 `PandaX/resource/pandax_iot.sql`，否则重新导入后功能异常。
- EMQX 容器重建后需要重新加载 exhook（`PandaX/deploy/emqx-exhook.conf`），否则设备消息进不来。
- 不要提交运行时产物：`pandax-run.err`、`pandax-run.log`、`vite-dev.log`、`bridge.log/err`。
- 源文件为 UTF-8；PowerShell 读取中文需加 `-Encoding UTF8`，否则显示乱码（仓库内历史注释本身也有乱码，不要顺带「修」）。

## Git 协作（二次开发）

上游仓库各自独立，均已配置「双远程 + dev 分支」：

| 目录 | origin（自己的 fork） | upstream（官方） | 开发分支 |
|---|---|---|---|
| `PandaX/` | `ssh://git@ssh.github.com:443/zaitian/PandaX.git` | `https://gitee.com/XM-GO/PandaX.git` | `dev` |
| `PandaUi/` | `ssh://git@ssh.github.com:443/zaitian/PandaXUi.git` | `https://gitee.com/XM-GO/PandaUi.git` | `dev` |
| `device-sdk/` | `ssh://git@ssh.github.com:443/zaitian/device-sdk-go.git` | `https://gitee.com/XM-GO/device-sdk.git` | `dev` |
| `screen/` | `ssh://git@ssh.github.com:443/zaitian/PandaXScreen.git` | `https://gitee.com/XM-GO/screen.git` | `dev` |
| `zigbee-bridge/` | `ssh://git@ssh.github.com:443/zaitian/zigbee-bridge.git` | 无（自研） | `main` |

- 直接用 HTTPS 连 GitHub 会 TLS 失败，必须使用上面的 `ssh.github.com:443` 形式（本机已配好 SSH key）。
- `master`/`main` 只跟随官方，自己的改动一律提交到 `dev`（`zigbee-bridge` 为自研项目，直接提交 `main`）。
- 同步官方更新：`git fetch upstream` → `git checkout master` → `git merge --ff-only upstream/master` → `git push origin master` → `git checkout dev` → `git merge master`。
- `device-sdk` 的 fork 与 gitee 官方在同一处网关主题修复上做了不同提交（fork: `3d7821b` 同时修了 telemetry/attribute；gitee: `05c3773`），合并官方更新时保留 fork 版本即可。
- 工作区级文档已随本仓库管理：`docs/architecture.md`（结构联系图）、`docs/workspace.md`（本文件）。

