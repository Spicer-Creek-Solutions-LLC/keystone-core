---
title: "Support"
weight: 4
description: >
  Get help with Keystone Core - community support, documentation, and troubleshooting resources
---

This page provides resources for getting help with Keystone Core, including community channels, documentation, and troubleshooting guides.

## Getting Help

### Before Asking

Before reaching out for help, please:

1. **Check the documentation**: Most questions are answered in the docs
2. **Search existing issues**: Your question may already be answered
3. **Review troubleshooting guides**: Common problems have documented solutions
4. **Gather information**: Have logs, config, and version info ready

### Quick Links

| Resource | Purpose | Link |
|----------|---------|------|
| Documentation | User guides and reference | [docs.keystonecore.io](/) |
| GitHub Issues | Bug reports and feature requests | [github.com/shawnbutts/keystone-core/issues](https://github.com/shawnbutts/keystone-core/issues) |
| GitHub Discussions | Q&A and community discussions | [github.com/shawnbutts/keystone-core/discussions](https://github.com/shawnbutts/keystone-core/discussions) |

---

## Community Support Channels

### GitHub Discussions

**Best for**: Questions, how-to help, feature discussions

GitHub Discussions is the primary community support channel:

- **Q&A**: Ask questions and get answers from the community
- **Ideas**: Discuss feature ideas before creating issues
- **Show & Tell**: Share your Keystone Core implementations
- **General**: General discussions about the project

**How to ask a good question:**

```markdown
## Question: [Brief description]

### What I'm trying to do
[Explain your goal]

### What I've tried
[List approaches you've attempted]

### Environment
- Keystone Core version: v0.9.0
- OS: Ubuntu 22.04
- Deployment: Single-node with embedded NATS

### Configuration
```yaml
# Relevant config sections
server:
  listen_address: ":8080"
```

### Logs
```
[Relevant log output]
```

### Expected vs Actual
- Expected: [what should happen]
- Actual: [what actually happens]
```

### Discord

**Best for**: Real-time help, quick questions, community chat

Join our Discord server for real-time support:

- **#general**: General discussion
- **#help**: Get help with issues
- **#feature-requests**: Discuss new features
- **#showcase**: Share your projects
- **#development**: Contributor discussions

**Discord etiquette:**
- Search before asking (your question may be answered)
- Be patient (not everyone is online 24/7)
- Use code blocks for logs and config
- Don't ping maintainers unless urgent

### Stack Overflow

**Best for**: Technical Q&A with long-term searchability

Ask questions on Stack Overflow with the tag `keystone-core`:

- Questions are indexed and searchable
- Answers are community-curated
- Great for specific technical problems

---

## Documentation Resources

### User Documentation

| Section | Description |
|---------|-------------|
| [Getting Started](/docs/getting-started/) | Installation, quick start, architecture |
| [Core Concepts](/docs/concepts/) | In-depth explanations of all subsystems |
| [Reference](/docs/reference/) | API, CLI, configuration, modules |
| [Operations](/docs/operations/) | Deployment, monitoring, maintenance |

### Troubleshooting Guides

Common issues and solutions:

| Guide | Topics Covered |
|-------|----------------|
| [Agent Troubleshooting](/docs/operations/troubleshooting/#agent-connectivity-issues) | Connection, registration, heartbeat issues |
| [NATS Troubleshooting](/docs/operations/troubleshooting/#nats-connection-problems) | Cluster, JetStream, performance issues |
| [State Troubleshooting](/docs/operations/troubleshooting/#state-application-failures) | Syntax, dependencies, execution failures |
| [Performance Tuning](/docs/operations/troubleshooting/#performance-issues) | CPU, memory, database optimization |

### Debug Logging

Enable debug logging for more information:

```bash
# Server
kscore-server --log-level=debug

# Agent
kscore-agent --log-level=debug

# CLI tools
kscorectl --log-level=debug exec run ...
```

Environment variable:
```bash
export KSCORE_LOG_LEVEL=debug
```

---

## Reporting Issues

### Bug Reports

Found a bug? Report it on GitHub:

**[Create a Bug Report](https://github.com/shawnbutts/keystone-core/issues/new?template=bug_report.md)**

**Required information:**
- Clear title describing the bug
- Steps to reproduce
- Expected behavior
- Actual behavior
- Environment details (OS, version, deployment type)
- Relevant logs

**Good bug report example:**

```markdown
## Bug: State apply fails with "module not found" for service module

### Environment
- Keystone Core: v0.9.0
- OS: CentOS 8
- Deployment: Single-node, embedded NATS

### Steps to reproduce
1. Create state file:
```yaml
webserver:
  service:
    - name: nginx
      state: running
```
2. Run `kscorectl state apply webserver.yaml`

### Expected
Service state should be applied

### Actual
Error: "module 'service' not found"

### Logs
```
2024-12-27 10:15:23 ERROR module not found name=service
```

### Notes
- File module works fine
- Package module works fine
- Only service module fails
```

### Feature Requests

Have an idea? Request it on GitHub:

**[Create a Feature Request](https://github.com/shawnbutts/keystone-core/issues/new?template=feature_request.md)**

**Include:**
- Use case description
- Proposed solution
- Alternatives considered
- Additional context

### Security Issues

**Do NOT report security vulnerabilities publicly.**

For security issues, please:

1. **Email**: security@keystonecore.io
2. **Include**: Detailed description, reproduction steps, impact assessment
3. **Wait**: We'll respond within 48 hours

See our [Security Policy](https://github.com/shawnbutts/keystone-core/security/policy) for more details.

---

## Self-Help Resources

### Common Issues

#### Agent Won't Connect

**Symptoms**: Agent fails to register, connection timeout

**Solutions**:
1. Check network connectivity: `telnet <server> 4222`
2. Verify server is running: `curl http://<server>:8080/health`
3. Check TLS certificates if using mTLS
4. Review firewall rules (port 4222 for NATS, 8080 for HTTP)

```bash
# Debug agent connection
kscore-agent --server-url=nats://server:4222 --log-level=debug

# Check server connectivity
nc -zv server 4222
```

#### State Apply Fails

**Symptoms**: State application errors, timeout

**Solutions**:
1. Validate state file syntax: `kscorectl state check <file>`
2. Check module availability: `kscorectl state list-modules`
3. Verify agent connectivity
4. Review dependency graph for cycles

```bash
# Check state file syntax
kscorectl state check mystate.yaml

# Dry-run to see what would change
kscorectl state apply --dry-run mystate.yaml
```

#### High Memory Usage

**Symptoms**: Server or agent using excessive memory

**Solutions**:
1. Check event storage size
2. Review retention policies
3. Tune JetStream memory limits
4. Check for memory leaks in custom modules

```bash
# Check event storage
curl http://localhost:8080/api/v1/events/stats

# Apply retention policy
kscorectl events retention --max-age=7d --apply
```

### Health Checks

Check system health:

```bash
# Server health
curl http://localhost:8080/health/live
curl http://localhost:8080/health/ready
curl http://localhost:8080/health/status

# Agent health
curl http://localhost:8081/health
```

### Version Information

Get version information:

```bash
# All components
kscorectl version

# Specific component
kscore-server version
kscore-agent version

# Detailed version
kscorectl version --verbose
```

---

## Commercial Support

### Enterprise Support

For organizations requiring guaranteed support SLAs:

- **Priority response times**: 4-hour response for critical issues
- **Dedicated support channel**: Private Slack/Teams integration
- **Architecture reviews**: Expert guidance on deployment
- **Training**: Custom training sessions for your team
- **Custom development**: Feature prioritization and custom integrations

**Contact**: enterprise@keystonecore.io

### Professional Services

Available professional services:

| Service | Description |
|---------|-------------|
| **Architecture Review** | Expert review of your deployment architecture |
| **Migration Assistance** | Help migrating from Salt/Ansible/Puppet |
| **Custom Module Development** | Build custom state modules for your needs |
| **Performance Tuning** | Optimize for your scale and workload |
| **Training Workshops** | On-site or virtual training sessions |

**Contact**: services@keystonecore.io

---

## Community Guidelines

### Code of Conduct

All community interactions are governed by our [Code of Conduct](https://github.com/shawnbutts/keystone-core/blob/main/CODE_OF_CONDUCT.md).

**Key principles:**
- Be respectful and inclusive
- Be patient with newcomers
- Focus on constructive feedback
- No harassment or discrimination

### Getting the Most from Support

**Do:**
- Provide detailed information upfront
- Use code blocks for logs and config
- Be patient and respectful
- Follow up on your issues
- Thank helpers

**Don't:**
- Ask for help without searching first
- Ping maintainers for general questions
- Cross-post the same question everywhere
- Expect immediate responses
- Be rude or demanding

---

## Contributing to Support

### Help Others

You can contribute by helping others:

1. **Answer questions** on GitHub Discussions
2. **Respond to issues** with solutions you've found
3. **Improve documentation** based on common questions
4. **Share knowledge** by writing articles about solving specific problems

### Become a Community Expert

Active community members can become recognized experts:

- **Contributor badge**: After significant contributions
- **Community Champion**: For exceptional community support
- **Maintainer**: For ongoing, dedicated contribution

---

## Feedback

### Documentation Feedback

Found an error or have a suggestion?

- **Edit on GitHub**: Use the "Edit this page" link
- **Open an issue**: [Documentation Issues](https://github.com/shawnbutts/keystone-core/issues/new?labels=documentation)
- **Discuss**: Join `#documentation` on Discord

### Product Feedback

Share your experience:

- **Feature requests**: GitHub Issues
- **General feedback**: GitHub Discussions or Discord
- **User research**: Contact us to participate in user research

---

## Contact

| Channel | Purpose | Response Time |
|---------|---------|---------------|
| GitHub Discussions | Community Q&A | 1-3 days |
| Discord | Real-time chat | Minutes to hours |
| GitHub Issues | Bug reports, features | 1-5 days |
| security@keystonecore.io | Security issues | 48 hours |
| enterprise@keystonecore.io | Enterprise inquiries | 1 business day |
| services@keystonecore.io | Professional services | 1 business day |
