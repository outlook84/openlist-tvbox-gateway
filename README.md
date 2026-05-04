# openlist-tvbox-gateway

![openlist-tvbox screenshot](screenshot.png)

`openlist-tvbox` 是一个面向 TVBox / CatVodSpider 的 OpenList / AList 中转网关。

它把服务端配置好的 OpenList / AList 网盘内容转换成 TVBox 可识别的分类、目录、搜索、播放数据。TVBox 客户端只访问本项目提供的网关接口，不直接接触 OpenList API key、账号密码或登录 token。

English documentation: [README.en.md](README.en.md)

## 功能特性

- 支持 OpenList / AList v3 访问。
- 支持匿名、API key、账号密码三种后端认证方式。
- 支持多个 OpenList / AList 后端。
- 支持多个 TVBox 订阅入口，每个订阅可以挂载不同后端、不同路径。
- 支持目录浏览、排序筛选、详情页播放列表、搜索和播放地址解析。
- 支持同目录字幕识别并随播放结果返回。
- 支持给订阅配置数字访问码，避免订阅地址被随意使用。
- 支持 Web Admin UI，通过浏览器维护 JSON 配置、测试后端连通性并查看运行日志。
- 内置 TVBox Spider JavaScript，订阅配置可直接引用网关内置脚本。
- 网关只开放明确的 TVBox 专用接口，不提供任意 OpenList API 转发或任意 URL 代理。

## 已测试 App 壳

以下 App 壳已完成测试：

- [takagen99/Box](https://github.com/takagen99/Box)
- [FongMi/TV](https://github.com/FongMi/TV)

## 部署方式

### 使用发布包

从项目 Release 下载与你的系统匹配的压缩包，解压后得到 `openlist-tvbox` 或 `openlist-tvbox.exe`。

从 [config.example.yaml](config.example.yaml) 复制一份为 `config.yaml`，按需修改后端和订阅入口，然后启动：

```bash
./openlist-tvbox -config config.yaml -listen :18989
```

### 启用 Web Admin UI

Web Admin UI 仅在 `-config` 指向 JSON 配置文件时启用，访问地址为：

```text
http://你的网关地址:18989/admin
```

首次启动时，如果未提供管理员访问码，网关会在配置文件同目录生成 `admin_setup_code`。打开 `/admin` 后输入这个 setup code，并设置一个 8 到 64 位、不能包含空白字符的管理员访问码。初始化完成后，网关会在同目录写入 `admin_access_code_hash`，并删除 `admin_setup_code`。

也可以用环境变量预置管理员访问码，适合容器或自动化部署：

```bash
OPENLIST_TVBOX_ADMIN_ACCESS_CODE='请换成强访问码' ./openlist-tvbox -config config.json -listen :18989
```

或者预置 bcrypt hash：

```bash
OPENLIST_TVBOX_ADMIN_ACCESS_CODE_HASH='$2a$...' ./openlist-tvbox -config config.json -listen :18989
```

Admin UI 会直接写入 JSON 配置文件，因此配置目录必须可写。使用 YAML 配置时网关仍可正常提供 TVBox 接口，但不会挂载 `/admin`；如果需要把不含 `api_key_env`、`password_env` 等环境变量密钥引用的 YAML 配置切到 Admin UI 管理，可以先导出 JSON：

```bash
./openlist-tvbox -config config.yaml -print-config-json > config.json
./openlist-tvbox -config config.json -listen :18989
```

注意：Admin UI 使用的可编辑 JSON 配置不支持 `api_key_env`、`password_env` 这类环境变量密钥引用；需要在 UI 中保存密钥。请限制 `/admin` 的公网访问范围，建议放在 HTTPS 反向代理后，并在反代场景设置 `public_base_url`；如需信任 `X-Forwarded-Proto`，同时设置 `trust_x_forwarded_for: true` 或在 UI 中开启对应选项。

如果从源码自行构建，请使用 `pnpm build:go` 或先执行 `pnpm build` 再执行 Go 构建，确保 Admin UI 前端资源被写入 `internal/admin/assets` 并嵌入二进制。

### 容器部署

示例运行参数：

```bash
docker run -d \
  --name openlist-tvbox \
  -p 18989:18989 \
  -v /path/to/config.yaml:/config/config.yaml:ro \
  ghcr.io/outlook84/openlist-tvbox-gateway:latest
```

容器中启用 Admin UI 时，请挂载整个配置目录，并确保运行用户对该目录可读写。

```bash
docker run -d \
  --name openlist-tvbox \
  -p 18989:18989 \
  -v /path/to/openlist-tvbox:/config \
  -e OPENLIST_TVBOX_CONFIG=config.json \
  ghcr.io/outlook84/openlist-tvbox-gateway:latest
```

启动后访问 `http://你的网关地址:18989/admin`。首次初始化需要的 `admin_setup_code` 位于宿主机 `/path/to/openlist-tvbox/admin_setup_code`。

## 接入 TVBox

`/sub` 是默认订阅入口。如果你在配置中定义了多个订阅入口，每个 `subs[].path` 都是一个独立订阅 URL，例如：

```text
http://你的网关地址:18989/sub
http://你的网关地址:18989/sub/movies
http://你的网关地址:18989/sub/shows
```

TVBox 会从订阅中加载内置 Spider 脚本，后续的分类、目录、搜索和播放请求都会回到网关处理。

反向代理、NAT、CDN 场景建议在配置中设置 `public_base_url`，确保 TVBox 拿到的是外部可访问地址。

## 访问码

订阅访问码使用 bcrypt hash 保存。

生成访问码 hash：

```bash
./openlist-tvbox -hash-password 123456
```

容器部署时可以在已启动容器中执行：

```bash
docker exec openlist-tvbox openlist-tvbox -hash-password 123456
```

然后把输出结果填写到对应订阅的 `access_code_hash`。访问码必须是 4 到 12 位数字，适配 TVBox 侧数字键盘输入。

已在 TVBox 客户端保存的访问码不会随删除订阅自动清除；如需让旧访问码失效，请更换订阅的访问码或清理客户端应用数据。

## 配置说明

建议从示例配置复制后修改，完整字段说明也在示例配置中：

- [config.example.yaml](config.example.yaml)
- [config.example.en.yaml](config.example.en.yaml)

常用配置入口：

- `public_base_url`：TVBox 看到的网关外部地址。
- `backends`：真实 OpenList / AList 服务配置。
- `subs`：对 TVBox 暴露的订阅入口。
- `subs[].mounts`：把后端路径挂载成 TVBox 分类。
- `access_code_hash`：订阅访问码 hash。

配置文件支持热重载。网关启动后会自动监视 `-config` 指定的配置文件，文件内容发生变化并通过校验后，会在不中断服务的情况下切换到新配置；如果新配置加载或校验失败，网关会记录错误并继续使用当前有效配置。

## 安全说明

- OpenList API key、账号密码和登录 token 只保存在网关服务端。

## 常用命令
打印起始配置：

```bash
./openlist-tvbox -print-config-example
```

指定配置文件和监听地址：

```bash
./openlist-tvbox -config config.yaml -listen :18989
```

健康检查：

```text
http://your-ip:18989/healthz
```
