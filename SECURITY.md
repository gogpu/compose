# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

**DO NOT** open a public GitHub issue for security vulnerabilities.

Instead, please report security issues via:

1. **Private Security Advisory** (preferred):
   https://github.com/gogpu/compose/security/advisories/new

2. **GitHub Discussions** (for less critical issues):
   https://github.com/gogpu/gogpu/discussions

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Affected versions
- Potential impact

### Response Timeline

- **Initial Response**: Within 72 hours
- **Fix & Disclosure**: Coordinated with reporter

## Security Considerations

compose uses IPC mechanisms for inter-process communication. Users should be aware of:

1. **Unix Domain Sockets** — socket files are created with default permissions. Use appropriate file permissions in production.
2. **Shared Memory** (Phase 2) — memory-mapped regions are shared between processes. Ensure only trusted modules connect.
3. **Wire Protocol** — frame data is not encrypted. For untrusted networks, use TLS or a secure transport.
4. **Module Identity** — module names are self-declared. The compositor should validate module identity for security-critical deployments.

## Security Contact

- **GitHub Security Advisory**: https://github.com/gogpu/compose/security/advisories/new
- **Public Issues**: https://github.com/gogpu/compose/issues

---

**Thank you for helping keep gogpu/compose secure!**
