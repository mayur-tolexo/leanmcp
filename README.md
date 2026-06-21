# leanmcp

A transparent MCP-to-MCP proxy that slims heavy tool-call responses to cut LLM token
usage — losslessly, with expand-on-demand. Put it in front of any MCP server and pay
fewer tokens for the same results.

## Why

MCP tool-call results are injected verbatim into a model's context. Data-heavy responses
(list/search endpoints especially) dominate token usage — a single 100-row JSON response
is easily ~15K tokens — and the cost is paid on every call. leanmcp reduces that cost
**without modifying any existing MCP server or API**.

## How it works

leanmcp speaks MCP on both sides: it is an MCP **server** to your client and an MCP
**client** to one upstream MCP server. Point your client at leanmcp instead of the
upstream — nothing else changes.

For each tool call it forwards to the upstream, then:

- **small results pass through untouched** — only heavy responses are optimized;
- **heavy results** are losslessly compacted (e.g. arrays of uniform objects become a
  single header + rows instead of repeated JSON keys), and the full raw result is cached
  behind an identity-scoped handle with a short TTL. The model can retrieve the full
  payload — or just a sub-path — on demand via an injected `expand_result` tool.

```
Client ──MCP──▶ leanmcp ──MCP(+caller auth)──▶ upstream MCP ──▶ backends
                   │  small result: pass through
                   │  heavy result: compact view + handle ──▶ Redis(raw, short TTL)
Client ──expand_result(handle, path?)──▶ leanmcp ──▶ Redis ──▶ requested slice
```

## Design principles

- **Lossless first.** v1 compaction never drops data without a handle to recover it.
  LLM summarization is deliberately excluded (roughly break-even on tokens, adds latency).
- **No API changes.** Pure infrastructure in front of any MCP server.
- **Fail open.** Any internal failure (store down, oversized payload, etc.) returns the
  full, correct result — optimization is best-effort, never a correctness risk.
- **Credential-bound handles.** A cache handle is bound to a keyed hash of the caller's
  credential and can be redeemed only by presenting the same credential — so a leaked
  handle is useless to anyone else. leanmcp never verifies or interprets the credential
  (it just forwards it upstream), so it works with any auth system out of the box.
- **Pluggable storage.** The cache is a narrow `Store` interface (Redis + in-memory
  reference impls); bring your own backend by implementing two methods.

## Run it

**Required environment variables:**

| Variable | Description |
|---|---|
| `LEANMCP_UPSTREAM_MCP_URL` | URL of the upstream MCP server (e.g. `http://upstream:8080/mcp`) |
| `LEANMCP_CACHE_SECRET` | HMAC key that binds expand handles to the caller credential — use a random string in production |

**Optional (Redis-backed store):**

| Variable | Description |
|---|---|
| `LEANMCP_STORE_TYPE` | Set to `redis` to use Redis; defaults to `memory` (single-instance only) |
| `LEANMCP_REDIS_URL` | Redis connection URL, e.g. `redis://redis:6379` |

**Start the proxy:**

```sh
LEANMCP_UPSTREAM_MCP_URL=http://upstream:8080/mcp \
LEANMCP_CACHE_SECRET=dev-secret \
go run ./cmd/leanmcp
```

Point your MCP client at `http://localhost:8080/mcp`. The proxy is transparent — no client
changes are required.

**Observability:**

Prometheus metrics are served at `http://localhost:8080/metrics`. Scrape this endpoint to
track token savings, cache hit rates, and latency.

**Start with shadow mode:**

Set `LEANMCP_SHADOW_MODE=true` (or `shadow_mode: true` in `leanmcp.example.yaml`) to compact
results and measure savings *without* returning handles to the model. This lets you validate
the token reduction in your environment before enabling handle-only mode.

## Status

Implemented and tested; not yet validated in production. The proxy, lossless tabular
compaction, credential-bound expand handles, the pluggable store (in-memory + Redis),
Prometheus metrics, and container/Kubernetes packaging are all in place, with unit
tests across packages. Roll out in shadow mode first (the default) and measure before
enabling handle-only mode. See [`docs/design`](docs/design) for the full design,
token-savings analysis, and rollout plan.

## License

Apache-2.0. See [LICENSE](LICENSE).
