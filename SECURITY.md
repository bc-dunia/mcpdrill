# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue, please report it responsibly.

### How to Report

1. **Do NOT** open a public GitHub issue for security vulnerabilities
2. Email security concerns to: [security@example.com](mailto:security@example.com)
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Any suggested fixes (optional)

### What to Expect

- **Acknowledgment**: Within 48 hours
- **Initial Assessment**: Within 1 week
- **Resolution Timeline**: Depends on severity
  - Critical: 1-7 days
  - High: 1-2 weeks
  - Medium: 2-4 weeks
  - Low: Next release cycle

### Scope

The following are in scope for security reports:

- Authentication/authorization bypasses
- Data exposure vulnerabilities
- Injection attacks (SQL, command, etc.)
- Denial of service vulnerabilities
- Configuration issues that compromise security

### Out of Scope

- Social engineering attacks
- Physical security issues
- Vulnerabilities in dependencies (report to upstream)
- Issues already reported and being addressed

## Security Best Practices

When deploying MCP Drill:

### Network Security

- Run the control plane behind a firewall or VPN
- Use TLS for all production connections
- Restrict API access with authentication tokens

### Agent Security

- Use unique, strong agent tokens
- Rotate tokens periodically
- Monitor agent connections for anomalies

### Configuration

- Never commit tokens or secrets to version control
- Use environment variables for sensitive configuration
- Review and restrict file system access

## Acknowledgments

We appreciate responsible disclosure and will acknowledge security researchers who help improve MCP Drill's security (with permission).
