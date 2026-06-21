# Security Policy

## Reporting a vulnerability

Please report security vulnerabilities **privately**. Do not open a public issue.

Use GitHub's [private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)
on this repository (the **Security** tab → **Report a vulnerability**).

Please include:

- A description of the issue and its impact.
- Steps to reproduce, or a proof of concept.
- Affected version/commit and configuration (e.g. upstream MCP server, auth setup).

We aim to acknowledge reports within a few business days and will keep you updated on
remediation progress. Once a fix is available, we will coordinate disclosure with you.

## Scope and threat model

leanmcp is a transparent proxy that caches full tool-call results to support
expand-on-demand. Security-relevant properties we care about:

- **Handle isolation.** Cached results are namespaced to a verified caller identity. A
  cache handle must never be usable by a different caller. Reports of cross-identity
  handle access are high severity.
- **Credential handling.** leanmcp forwards the caller's credential to the upstream and
  verifies it to derive identity. It must not log, persist, or leak credentials.
- **Cache lifetime.** Cached raw results use a short TTL and are evicted; reports of
  unbounded retention or data surviving credential revocation are in scope.
- **Fail-open behavior.** Optimization failures must not change authorization outcomes —
  leanmcp adds no new authorization decisions beyond identity verification for handle
  scoping.

Out of scope: vulnerabilities in upstream MCP servers, the MCP client, or Redis itself
(report those to their respective projects), and issues requiring a compromised host.

## Supported versions

leanmcp is pre-1.0. Security fixes are applied to `main`. Pin to a commit or release tag
for reproducible deployments.
