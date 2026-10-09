# Security Policy

## Supported versions

Security fixes are released for the latest `0.1.x` release only. Please upgrade to the [latest release](https://github.com/faucetdb/faucet/releases/latest) before reporting.

| Version | Supported |
|---------|-----------|
| Latest 0.1.x release | Yes |
| Older releases | No |

## Reporting a vulnerability

Please do **not** open a public issue for security problems. Report them privately, either way:

- Email **developers@faucetdb.ai**
- Use GitHub's [private vulnerability reporting](https://github.com/faucetdb/faucet/security/advisories/new)

Include the Faucet version (`faucet version`), the database driver involved, steps to reproduce, and the impact you expect (for example: an API key reading a table its role does not allow).

We will acknowledge your report, keep you updated while we work on a fix, and credit you in the release notes unless you prefer not to be named.

## Scope

Examples of issues we want to hear about:

- Bypasses of role-based access control (REST API or MCP tools)
- Authentication problems with API keys or admin sessions
- SQL injection through filters, ordering, field selection or MCP tool arguments
- Exposure of stored database credentials or API keys

Running `faucet mcp` in stdio mode with admin rights, and row-level filters not being enforced yet, are documented behavior rather than vulnerabilities.
