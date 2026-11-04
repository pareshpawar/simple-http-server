# Simple HTTP Server in Go as a Container Image
[![ci](https://github.com/pareshpawar/simple-http-server/actions/workflows/main.yml/badge.svg)](https://github.com/pareshpawar/simple-http-server/actions/workflows/main.yml)

`simple-http-server` echoes back everything about the request it receives: IP addresses, headers, form data, body and more. Use it as a test app to practice deployments, or to debug proxies, ingresses, load balancers and service meshes in Docker or Kubernetes.

## Quick start

The image is on [Docker Hub](https://hub.docker.com/r/pareshpawar/simple-http-server) for `linux/amd64` and `linux/arm64`:

```bash
docker run --rm -p 8081:8081 pareshpawar/simple-http-server
curl localhost:8081
```

Or build and run it yourself (Go 1.22+):

```bash
make run          # or: go run .
make test         # run the test suite
make docker       # build the image locally
```

## What it returns

Every echo response includes:

- Request time, method, URL and protocol
- Host header, remote address and client IP (honours `X-Forwarded-For` / `X-Real-IP`)
- The server's local IP, hostname (pod or container name) and version
- All request headers
- Query and form data, plus the request body (up to 1 MiB)
- The `YOUR_ENV` environment variable, plus any variables listed in `ECHO_ENV`

Every response also carries `X-Served-By: <hostname>` and `X-Server-Version` headers, so you can see which replica answered when testing load balancing.

## Endpoints

| Endpoint | Description |
| --- | --- |
| `/` (any path) | Echo in the default format (`text`, unless `OUTPUT_FORMAT` says otherwise). Use `?format=json\|html\|text` or `Accept: application/json` to choose per request. |
| `/json` | Echo as JSON. |
| `/html/` | Echo as an HTML page. |
| `/static/...` | Files from `STATIC_DIR`, when set. |
| `/status/{code}` | Responds with the given status code (200–599), e.g. `/status/503`. |
| `/delay/{duration}` | Waits before responding, e.g. `/delay/500ms`, `/delay/2s` or `/delay/3` (seconds). Max 60s. |
| `/healthz`, `/healthcheck` | Liveness probe: always `200 OK`. |
| `/readyz` | Readiness probe: `503` until the server is listening and again once shutdown starts. |
| `/version` | Version and hostname as JSON. |

Examples:

```bash
curl localhost:8081/json
curl -X POST -d 'name=paresh' localhost:8081/
curl -H 'Accept: application/json' localhost:8081/anything?x=1
curl -i localhost:8081/status/418
curl localhost:8081/delay/2s
```

## Configuration

Every option can be set with an environment variable or a flag (flags win).

| Env var | Flag | Default | Description |
| --- | --- | --- | --- |
| `PORT` | `-port` | `8081` | Port to listen on. |
| `BIND_ADDRESS` | `-bind` | `0.0.0.0` | Address to listen on. |
| `OUTPUT_FORMAT` | `-format` | `text` | Default format for `/`: `text`, `html` or `json`. |
| `STATIC_DIR` | `-static-dir` | (empty) | Directory to serve under `/static/`. Disabled when empty. |
| `TEMPLATE_FILE` | `-template` | `html/index.html` | HTML template for `/html/`. If the default path doesn't exist, the template built into the binary is used. |
| `ECHO_ENV` | `-echo-env` | (empty) | Comma-separated extra env vars to include in responses (e.g. `POD_NAME,NODE_NAME`). |
| `YOUR_ENV` | | (empty) | Always echoed back. |
| `SHUTDOWN_DELAY` | `-shutdown-delay` | `0s` | How long to keep serving after SIGTERM while `/readyz` reports 503, so load balancers can drain traffic. |
| `NO_COLOR` | `-no-color` | (unset) | Disable ANSI colors in logs and text responses. |

Other flags: `-version` prints the version, and `-healthcheck` probes the running server's `/healthz` and exits non-zero if it is unhealthy. The Docker image uses `-healthcheck` for its `HEALTHCHECK`.

### Serve your own files

```bash
docker run --rm -p 8081:8081 -v "$PWD/site:/srv:ro" -e STATIC_DIR=/srv \
  pareshpawar/simple-http-server
curl localhost:8081/static/index.html
```

### Custom HTML template

Mount your own template over `/html/index.html`. It is a Go [`html/template`](https://pkg.go.dev/html/template) and gets the request as `.Info`, with the same fields as the JSON output (`.Info.Method`, `.Info.Headers`, `.Info.ClientIP`, `.Info.Env`, ...). A `join` function is available, e.g. `{{ join $values ", " }}`. Older templates using `.Request.*` and `.Headers` still work.

```bash
docker run --rm -p 8081:8081 -v "$PWD/my.html:/html/index.html:ro" \
  pareshpawar/simple-http-server
```

## Kubernetes example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: simple-http-server
spec:
  replicas: 3
  selector:
    matchLabels: { app: simple-http-server }
  template:
    metadata:
      labels: { app: simple-http-server }
    spec:
      containers:
        - name: app
          image: pareshpawar/simple-http-server
          ports: [{ containerPort: 8081 }]
          env:
            - { name: SHUTDOWN_DELAY, value: "5s" }
            - { name: ECHO_ENV, value: "POD_NAME,NODE_NAME" }
            - name: POD_NAME
              valueFrom: { fieldRef: { fieldPath: metadata.name } }
            - name: NODE_NAME
              valueFrom: { fieldRef: { fieldPath: spec.nodeName } }
          livenessProbe:
            httpGet: { path: /healthz, port: 8081 }
          readinessProbe:
            httpGet: { path: /readyz, port: 8081 }
```

## To Do

- [x] Make std output/logs colored and pretty 😅
- [x] Serving html file as output on ```/html/``` endpoint
- [x] create github actions for docker image build
- [x] Add Environment Variable to switch text output to html output (`OUTPUT_FORMAT`)
- [x] Serve html pages from an external directory (`STATIC_DIR`)
- [x] Add volume Env var to serve volume as http dir (`STATIC_DIR`)
- [ ] Optional TLS support
