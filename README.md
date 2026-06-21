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

## Status

Pre-implementation. See [`docs/design`](docs/design) for the full design, token-savings
analysis, and rollout plan.

## License

Apache-2.0. See [LICENSE](LICENSE).
