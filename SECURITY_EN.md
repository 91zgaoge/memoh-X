# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest | ✅ Fully supported |
| Previous | ✅ Security updates |
| Older | ❌ No longer supported |

## Reporting a Vulnerability

The Memoh-v2 team takes security issues very seriously. If you discover a security vulnerability, please follow these steps:

### Reporting Process

1. **Do Not Disclose Publicly**: Please do not disclose security vulnerabilities publicly on GitHub Issues, discussions, or other public channels.

2. **Report Privately**: Please report privately using one of the following methods:
   - Email the project maintainers (find in GitHub profiles)
   - Use GitHub's [Private Vulnerability Reporting](https://github.com/91zgaoge/memoh-X/security/advisories/new) (recommended)

3. **Provide Details**: Include the following in your report:
   - Description and location of the vulnerability
   - Steps to reproduce
   - Potential impact scope
   - Suggested fix (if any)
   - Your contact information

4. **Wait for Response**: We will acknowledge receipt within 48 hours and provide an initial assessment within 7 business days.

### Response Process

1. **Acknowledge**: We acknowledge receipt immediately
2. **Assess Impact**: Analyze severity and scope
3. **Develop Fix**: Create a fix
4. **Release Update**: Publish security update
5. **Disclose**: Responsibly disclose after fix (with credit to reporter)

## Security Best Practices

### Deployment Security

1. **Stay Updated**: Always use the latest version and apply security updates promptly
2. **Strong Passwords**: Use strong passwords for all admin accounts
3. **Environment Isolation**: Production should be isolated from development
4. **Network Configuration**:
   - Only expose necessary ports
   - Use reverse proxy (e.g., Nginx) for SSL/TLS
   - Configure appropriate firewall rules

### API Key Management

1. **Secure Storage**: Don't hardcode API keys in code
2. **Least Privilege**: Configure minimum necessary permissions
3. **Regular Rotation**: Rotate API keys regularly
4. **Environment Variables**: Use environment variables for sensitive config

### Container Security

1. **Image Updates**: Regularly update base images
2. **Non-root User**: Run containers as non-root when possible
3. **Resource Limits**: Set CPU and memory limits
4. **Read-only Filesystem**: Use read-only filesystem where possible

### Data Protection

1. **Database Encryption**: Encrypt sensitive data at rest
2. **Backup Encryption**: Encrypt backup files
3. **Transit Encryption**: Use HTTPS for data transmission
4. **Regular Backups**: Backup regularly and verify recovery

## Known Security Issues

We will list currently known, unpatched security issues here:

_No known unpatched security issues at this time_

## Security Update History

| Date | Version | Description | CVE |
|------|---------|-------------|-----|
| - | - | No records | - |

## Acknowledgments

Thanks to the following people for their security contributions to Memoh-v2:

_List to be updated_

## Disclaimer

Memoh-v2 is provided "as is" without warranty of any kind. Use at your own risk.

## Contact

For security-related questions, contact us via:

- GitHub Security Advisories: https://github.com/91zgaoge/memoh-X/security
- Project Issues (non-security only): https://github.com/91zgaoge/memoh-X/issues

---

Last updated: 2026-03-11
