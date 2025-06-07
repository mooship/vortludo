# Security Policy

## Project Security

Vortludo is a libre Wordle clone that takes security seriously. We strive to keep the application secure for all users.

## Reporting a Vulnerability

If you discover a security vulnerability in Vortludo, please report it responsibly:

### How to Report
- **GitHub Issues**: For non-sensitive security issues, you can open a [GitHub issue](https://github.com/mooship/vortludo/issues)
- **Email**: For sensitive vulnerabilities, please email the maintainer directly
- **Security Advisories**: Use GitHub's [security advisory feature](https://github.com/mooship/vortludo/security/advisories) for responsible disclosure

### What to Include
Please include as much information as possible:
- Description of the vulnerability
- Steps to reproduce the issue
- Potential impact and attack scenarios
- Suggested fixes (if any)
- Your contact information for follow-up

### Response Timeline
- **Acknowledgment**: We aim to acknowledge receipt within 48 hours
- **Resolution**: Critical vulnerabilities will be prioritized and addressed as quickly as possible

### Security Scope
Areas of particular interest for security reports:
- Session management and authentication
- File upload/download security
- Input validation and sanitization
- Cross-site scripting (XSS) vulnerabilities
- SQL injection or file path traversal
- Denial of service attacks
- Data privacy issues

## Security Best Practices

When deploying Vortludo:
- Always use HTTPS in production
- Keep dependencies updated
- Use secure session cookies
- Implement proper file permissions
- Monitor logs for suspicious activity
- Follow the deployment guidelines in the README
