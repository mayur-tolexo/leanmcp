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

- **Handle isolation.** Each cache handle is bound to a keyed hash of the caller's
  credential and may be redeemed only by presenting the same credential. A handle must
  never be usable by a different caller. Reports of cross-caller handle redemption are
  high severity.
- **Credential handling.** leanmcp forwards the caller's credential to the upstream
  unchanged and computes a keyed hash of it solely to bind handles. It does not verify or
  interpret the credential, and must not log, persist, or leak the credential itself (only
  a hash prefix may appear in audit logs).
- **Cache lifetime.** Cached raw results use a short TTL and are evicted; reports of
  unbounded retention are in scope. Note the post-revocation window is bounded by the TTL
  (see the design doc, §8.1).
- **Fail-open behavior.** Optimization failures must not change outcomes — leanmcp makes
  no authorization decisions; the upstream remains the sole authority.

Out of scope: vulnerabilities in upstream MCP servers, the MCP client, or Redis itself
(report those to their respective projects), and issues requiring a compromised host.

## Supported versions

leanmcp is pre-1.0. Security fixes are applied to `main`. Pin to a commit or release tag
for reproducible deployments.
