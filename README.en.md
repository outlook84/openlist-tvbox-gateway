# openlist-tvbox-gateway

`openlist-tvbox` is a read-only OpenList / AList gateway for TVBox / CatVodSpider.

It converts server-side OpenList / AList content into TVBox-compatible categories, directory listings, search results, details, and playback data. TVBox clients talk only to this gateway and never receive your OpenList API key, username/password, or login token.

Simplified Chinese documentation: [README.md](README.md)

## Features

- Read-only OpenList / AList v3 access.
- Anonymous, API key, and username/password backend authentication.
- Multiple OpenList / AList backends.
- Multiple TVBox subscription endpoints, each with mounts from different backends and backend paths.
- Directory browsing, sorting filters, detail playlists, search, and playback URL resolution.
- Subtitle detection for files in the same directory.
- Optional numeric access code for subscription protection.
- Web Admin UI for editing JSON config, testing backend connectivity, and viewing runtime logs.
- Embedded TVBox Spider JavaScript served by the gateway.
- No arbitrary OpenList API forwarding and no arbitrary URL proxying.

## Tested App Shells

The following TVBox app shells have been tested:

- [takagen99/Box](https://github.com/takagen99/Box)
- [FongMi/TV](https://github.com/FongMi/TV)

## Deployment

### Release Binary

Download the archive for your operating system from the project releases, then extract `openlist-tvbox` or `openlist-tvbox.exe`.

Copy [config.example.en.yaml](config.example.en.yaml) to `config.yaml`, adjust the backends and subscription entries, then start the gateway:

```bash
./openlist-tvbox -config config.yaml -listen :18989
```

### Enable Web Admin UI

The Web Admin UI is enabled only when `-config` points to a JSON config file. Open:

```text
http://your-gateway-host:18989/admin
```

On first startup, if no admin access code is provided, the gateway creates `admin_setup_code` next to the config file. Open `/admin`, enter that setup code, and set an admin access code. The admin access code must be 8 to 64 characters and must not contain whitespace or control characters. After setup, the gateway writes `admin_access_code_hash` in the same directory and removes `admin_setup_code`.

You can also preconfigure the admin access code with an environment variable, which is useful for containers and automated deployments:

```bash
OPENLIST_TVBOX_ADMIN_ACCESS_CODE='replace-with-a-strong-code' ./openlist-tvbox -config config.json -listen :18989
```

Or preconfigure a bcrypt hash:

```bash
OPENLIST_TVBOX_ADMIN_ACCESS_CODE_HASH='$2a$...' ./openlist-tvbox -config config.json -listen :18989
```

The Admin UI writes directly to the JSON config file, so the config directory must be writable. YAML configs still work for TVBox gateway APIs, but `/admin` is not mounted for YAML. To move an existing YAML config that does not use env-backed secrets such as `api_key_env` or `password_env` to Admin UI management, export it as JSON first:

```bash
./openlist-tvbox -config config.yaml -print-config-json > config.json
./openlist-tvbox -config config.json -listen :18989
```

Note: editable JSON config used by Admin UI does not support env-backed secrets such as `api_key_env` or `password_env`; save secrets in the UI instead. Restrict public access to `/admin`; HTTPS behind a reverse proxy is recommended. In reverse-proxy deployments, set `public_base_url`; if you need to trust `X-Forwarded-Proto`, also set `trust_x_forwarded_for: true` or enable the matching option in the UI.

If you build from source, use `pnpm build:go`, or run `pnpm build` before the Go build, so the Admin UI frontend assets are written to `internal/admin/assets` and embedded into the binary.

### Container Deployment

Example:

```bash
docker run -d \
  --name openlist-tvbox \
  -p 18989:18989 \
  -v /path/to/config.yaml:/config/config.yaml:ro \
  ghcr.io/outlook84/openlist-tvbox-gateway:latest
```

To enable Admin UI in a container, mount the whole config directory instead of a single read-only config file, because the gateway needs to write `config.json`, `admin_setup_code`, and `admin_access_code_hash`:

```bash
docker run -d \
  --name openlist-tvbox \
  -p 18989:18989 \
  -v /path/to/openlist-tvbox:/config \
  -e OPENLIST_TVBOX_CONFIG=config.json \
  ghcr.io/outlook84/openlist-tvbox-gateway:latest
```

Then open `http://your-gateway-host:18989/admin`. The first-run `admin_setup_code` is available at `/path/to/openlist-tvbox/admin_setup_code` on the host.

## TVBox Setup

`/sub` is the default subscription endpoint. If your config defines multiple subscriptions, every `subs[].path` is an independent TVBox subscription URL, for example:

```text
http://your-gateway-host:18989/sub
http://your-gateway-host:18989/sub/movies
http://your-gateway-host:18989/sub/shows
```

TVBox loads the embedded Spider script from the subscription. Category, listing, search, detail, and play requests are then handled by the gateway.

If the gateway is behind a reverse proxy, NAT, or CDN, set `public_base_url` so TVBox receives the externally reachable URL.

## Access Code

Subscription access codes are stored as bcrypt hashes.

Generate a hash:

```bash
./openlist-tvbox -hash-password 123456
```

For container deployments, run it inside the started container:

```bash
docker exec openlist-tvbox openlist-tvbox -hash-password 123456
```

Put the output into `access_code_hash` for the subscription. The access code must be 4 to 12 digits so it can be entered with the TVBox numeric keypad.

Access codes saved by the TVBox client are not automatically removed when a subscription is deleted. To invalidate an old saved code, change the subscription access code or clear the client app data.

## Configuration

Start from an example config and edit it as needed. The example configs also contain the full field documentation:

- [config.example.yaml](config.example.yaml)
- [config.example.en.yaml](config.example.en.yaml)

Common fields:

- `public_base_url`: external gateway URL visible to TVBox.
- `backends`: real OpenList / AList backend definitions.
- `subs`: TVBox subscription endpoints.
- `subs[].mounts`: backend paths exposed as TVBox categories.
- `access_code_hash`: subscription access-code hash.

Config files support hot reload. After startup, the gateway watches the file specified by `-config`; when the file changes and the new config loads successfully, the gateway switches to it without interrupting service. If the new config fails to load or validate, the gateway logs the error and keeps using the current valid config.

## Security Notes

- OpenList API keys, passwords, and login tokens stay on the gateway server.

## Useful Commands

Print a starter config:

```bash
./openlist-tvbox -print-config-example
```

Set config file and listen address:

```bash
./openlist-tvbox -config config.yaml -listen :18989
```

Health check:

```text
http://your-ip:18989/healthz
```
