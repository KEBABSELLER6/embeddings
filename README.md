# embeddings

A small embeddings API in Go. It exposes one operation, "turn these strings
into vectors", over HTTP and gRPC, and forwards the work to
[Ollama](https://ollama.com) or any OpenAI-compatible embeddings endpoint.

It runs either as a standalone service or as a Kubernetes sidecar next to the
app that needs embeddings.

- HTTP and gRPC APIs with the same behaviour and error mapping
- Ollama and OpenAI-compatible providers
- Optional request chunking with parallel upstream calls
- Optional static API key auth
- Prometheus metrics, liveness and readiness probes on a separate admin port
- Graceful shutdown with a configurable drain delay

## Quick start

Requires Go 1.27 and Docker (for the local Ollama).

```sh
make dev-up                    # start Ollama and pull nomic-embed-text
cp .env.example .env           # adjust as needed
set -a && . ./.env && set +a   # the server reads plain environment variables
make run
```

```sh
curl localhost:8080/api/v1/embed -d '{"input":["hello","world"]}'
```

`make dev-rst` stops Ollama and deletes its volume.

## API

### HTTP

`POST /api/v1/embed`

```json
{ "input": ["hello", "world"] }
```

```json
{ "model": "nomic-embed-text", "embeddings": [[0.12, -0.03, ...], [...]] }
```

Embeddings are returned in the same order as the inputs. `input` must hold
between 1 and `MAX_INPUTS` strings, and the body must fit in `MAX_BODY_SIZE`.

Errors are JSON with a correlation id:

```json
{ "message": "invalid input", "correlation_id": "6f1c..." }
```

| Status | Meaning |
|---|---|
| 400 | Invalid JSON, no inputs, or more than `MAX_INPUTS` |
| 401 | API key missing or wrong |
| 413 | Body larger than `MAX_BODY_SIZE` |
| 499 | Client disconnected before the response was ready |
| 502 | The provider failed or returned something unusable |
| 504 | The provider did not answer within `UPSTREAM_TIMEOUT` |
| 500 | Anything unexpected |

### gRPC

`embeddings.v1.EmbeddingService/Embed`, defined in
[`proto/embeddings/v1/embed.proto`](proto/embeddings/v1/embed.proto).
Errors map to `InvalidArgument`, `Unauthenticated`, `ResourceExhausted`
(message larger than `MAX_BODY_SIZE`), `Canceled`, `DeadlineExceeded`,
`Unavailable` and `Internal`, mirroring the HTTP table.

The standard `grpc.health.v1.Health` service is always registered. Server
reflection is off unless `GRPC_REFLECTION=true`.

### Correlation ids

Send `X-Correlation-ID` (HTTP header or gRPC metadata) to tag a request;
otherwise one is generated. It is returned in the response and included in
every log line for that request.

### Authentication

When `API_KEY` is set, every embed call must send it in `X-API-Key` (HTTP
header) or `x-api-key` (gRPC metadata). Health checks and the admin port never
require it. When `API_KEY` is empty, auth is off.

```sh
curl -H 'X-API-Key: <key>' localhost:8080/api/v1/embed -d '{"input":["hello"]}'
```

## Ports

| Port | Default | Serves | Bound to |
|---|---|---|---|
| HTTP API | 8080 | `POST /api/v1/embed` | `LISTEN_HOST` |
| gRPC API | 50051 | `EmbeddingService`, health, reflection | `LISTEN_HOST` |
| Admin | 9090 | `GET /metrics`, `/healthz`, `/readyz` | all interfaces |

The admin port always listens on all interfaces so Prometheus and the kubelet
can reach it even when the API is bound to localhost.

- `/healthz` (liveness): 200 while the process is running.
- `/readyz` (readiness): 200 while serving, 503 once shutdown has started.

Metrics: `embeddings_requests_total` and `embeddings_request_duration_seconds`,
labelled by transport, route or gRPC method, and status. Plus the standard Go
and process collectors.

## Configuration

All configuration is through environment variables. Invalid values stop the
server at startup with an error naming the variable.

| Variable | Default | Description |
|---|---|---|
| `PROVIDER` | (required) | `ollama` or `openai` |
| `OLLAMA_ENDPOINT_URL` | (required for ollama) | e.g. `http://localhost:11434/api/embed` |
| `OLLAMA_MODEL` | (required for ollama) | e.g. `nomic-embed-text` |
| `OPENAI_BASE_URL` | (required for openai) | e.g. `https://api.openai.com/v1`; any OpenAI-compatible server works |
| `OPENAI_MODEL` | (required for openai) | e.g. `text-embedding-3-small` |
| `OPENAI_API_KEY` | (required for openai) | Key for the provider |
| `API_KEY` | empty (auth off) | Key clients must send; see [Authentication](#authentication) |
| `PORT` | `8080` | HTTP API port |
| `GRPC_PORT` | `50051` | gRPC API port |
| `METRICS_PORT` | `9090` | Admin port |
| `LISTEN_HOST` | empty (all interfaces) | Address the HTTP and gRPC API ports bind to, e.g. `127.0.0.1`. Does not affect the admin port |
| `MAX_INPUTS` | `2048` | Maximum strings per request |
| `MAX_BODY_SIZE` | `512000` | Maximum request size in bytes (HTTP body and gRPC message) |
| `BATCH_SIZE` | `0` (off) | Split requests into chunks of this many inputs; up to 4 chunks run in parallel |
| `UPSTREAM_TIMEOUT` | `30s` | Time limit for the provider, covering all chunks of a request |
| `DRAIN_DELAY` | `0s` | How long to keep serving after readiness turns 503; see [Shutdown](#shutdown) |
| `SHUTDOWN_TIMEOUT` | `35s` | Total shutdown budget. Must be larger than `DRAIN_DELAY + UPSTREAM_TIMEOUT` |
| `GRPC_REFLECTION` | `false` | Register gRPC server reflection |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `LOG_FORMAT` | `text` | `text` or `json` |

The ports must all be different.

## Shutdown

On `SIGTERM` or `SIGINT` the server:

1. Reports not ready: `/readyz` returns 503 and gRPC health returns
   `NOT_SERVING`. Liveness stays 200.
2. Keeps serving for `DRAIN_DELAY`, so load balancers can stop sending
   traffic. Skipped if the server failed to start.
3. Ends long-lived gRPC streams, such as health watches.
4. Stops HTTP and gRPC in parallel, letting in-flight requests finish, until
   `SHUTDOWN_TIMEOUT` (counted from step 1) runs out.
5. Stops the admin server.

Behind a Kubernetes Service, set `DRAIN_DELAY` a little longer than the
readiness probe needs to notice, e.g. `periodSeconds: 2` and
`failureThreshold: 2` → `DRAIN_DELAY=5s`, and keep
`terminationGracePeriodSeconds` at or above `SHUTDOWN_TIMEOUT`.

## Running as a sidecar

[`example/deployment.yaml`](example/deployment.yaml) is a working example: the
embeddings server runs next to a placeholder `app` container in the same pod,
backed by the dev stack's Ollama on the host.

- It is a [native sidecar](https://kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/)
  (`initContainers` with `restartPolicy: Always`, Kubernetes 1.29+). It starts
  before the app, and its startup probe holds the app back until it is ready.
  On shutdown the app stops first and the sidecar after it.
- `LISTEN_HOST=127.0.0.1` makes the API reachable only from inside the pod.
  The app calls `http://localhost:8080` or `localhost:50051`.
- Probes use the admin port, which the kubelet can reach.
- `DRAIN_DELAY=0s`: no Service routes to the sidecar and the app is already
  gone when it stops, so there is nothing to drain.
- The pod shares one `terminationGracePeriodSeconds` (default 30s), and the
  app uses part of it first, so the sidecar gets shorter timeouts.
- `OLLAMA_ENDPOINT_URL` uses `host.k3d.internal`, which k3d resolves to the
  host where compose publishes Ollama. On another cluster, point it at your
  Ollama Service, or switch `PROVIDER` to `openai` and add the `OPENAI_*`
  variables with the key from a secret.
- `GOMEMLIMIT` sits below the memory limit so the Go GC works harder before
  the container is OOM-killed. Large requests are memory-hungry: 2048 inputs
  of a 1536-dimension model is ~60 MB of JSON from the provider. Raise the
  limit, or lower `MAX_INPUTS` / use `BATCH_SIZE`.
- The container runs as a non-root user with a read-only filesystem and no
  capabilities.

To try it in [k3d](https://k3d.io/) with a local build:

```sh
make dev-up                                   # Ollama on host port 11434
k3d cluster create embeddings
docker build -t embeddings:dev .
k3d image import embeddings:dev -c embeddings
kubectl create secret generic embedder --from-literal=api-key=<key>
kubectl apply -f example/deployment.yaml

kubectl exec deploy/sidecar-poc -c app -- \
  curl -s -H 'X-API-Key: <key>' localhost:8080/api/v1/embed -d '{"input":["hello"]}'
```

`kubectl delete pod -l app=sidecar-poc` shows the shutdown order in the logs:
the app stops first, then the sidecar. `k3d cluster delete embeddings` cleans
up.

## Development

```sh
make test    # go test -race ./...
make vet     # go vet ./...
make build   # bin/server
make proto   # regenerate gen/ from proto/ (needs protoc and the Go plugins)
```

Released images are published to `ghcr.io/kebabseller6/embeddings` for
`linux/amd64` and `linux/arm64`, tagged `X.Y.Z` and `X.Y`.

### Upgrading from 0.3

`/healthz` and `/readyz` moved from the HTTP API port to the admin port
(`METRICS_PORT`, default 9090). Update any probes that pointed at 8080.

## License

See [LICENSE](LICENSE).
