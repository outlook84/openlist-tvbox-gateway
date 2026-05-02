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
- Embedded TVBox Spider JavaScript served by the gateway.
- No arbitrary OpenList API forwarding and no arbitrary URL proxying.

## Tested App Shells

The following TVBox app shells have been tested:

- [takagen99/Box](https://github.com/takagen99/Box)
- [FongMi/TV](https://github.com/FongMi/TV)

## Quick Start

1. Prepare a config file.

   Copy [config.example.en.yaml](config.example.en.yaml) to `config.yaml`, then adjust the backends and subscription entries as needed. See "Configuration" below for field notes.

2. Start the gateway.

   ```bash
   ./openlist-tvbox -config config.yaml -listen :18989
   ```

3. Add the subscription URL to TVBox.

   ```text
   http://your-gateway-host:18989/sub
   ```

   If the gateway is behind a reverse proxy, NAT, or CDN, set `public_base_url` so TVBox receives the externally reachable URL.

## Deployment

### Release Binary

Download the archive for your operating system from the project releases, extract it, and get `openlist-tvbox` or `openlist-tvbox.exe`. Then follow the quick start above to prepare the config and start the gateway.

### Container Deployment

Example:

```bash
docker run -d \
  --name openlist-tvbox-gateway \
  -p 18989:18989 \
  -v /path/to/config.yaml:/config/config.yaml:ro \
  ghcr.io/outlook84/openlist-tvbox:latest
```

## TVBox Setup

The `/sub` path in the quick start is the default subscription endpoint. If your config defines multiple subscriptions, every `subs[].path` is an independent TVBox subscription URL, for example:

```text
http://your-gateway-host:18989/sub
http://your-gateway-host:18989/sub/movies
http://your-gateway-host:18989/sub/shows
```

TVBox loads the embedded Spider script from the subscription. Category, listing, search, detail, and play requests are then handled by the gateway.

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
