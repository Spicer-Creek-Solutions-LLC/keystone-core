# LAMP Stack Blueprint

A production-ready LAMP (Linux, Apache, MySQL/MariaDB, PHP) stack blueprint for Keystone Core.

## Overview

This blueprint deploys a complete web application stack with:

- **Apache HTTP Server** - Configured with security headers and virtual hosts
- **PHP** - With common extensions and OPcache for performance
- **MySQL/MariaDB** - Secure installation with application database and user

## Quick Start

```yaml
# In your state file
include:
  - blueprint: blueprints/kscore/lamp-stack@1.0.0
    params:
      server_name: example.com
      document_root: /var/www/example.com
      mysql_root_password: !secret mysql/root_password
      mysql_password: !secret mysql/app_password
```

## Parameters

### Required Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `server_name` | string | Server hostname (e.g., example.com) |
| `mysql_root_password` | string | MySQL root password (use secrets!) |
| `mysql_password` | string | Application database password |

### Optional Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `server_admin` | string | webmaster@localhost | Admin email |
| `document_root` | string | /var/www/html | Web root directory |
| `http_port` | integer | 80 | HTTP port |
| `php_version` | string | 8.2 | PHP version to install |
| `php_memory_limit` | string | 256M | PHP memory limit |
| `php_max_execution_time` | integer | 30 | PHP max execution time |
| `mysql_database` | string | app_db | Application database name |
| `mysql_user` | string | app_user | Application database user |
| `mysql_max_connections` | integer | 100 | MySQL max connections |
| `mysql_bind_address` | string | 127.0.0.1 | MySQL bind address |

### Feature Flags

| Feature | Default | Description |
|---------|---------|-------------|
| `phpmyadmin` | false | Install phpMyAdmin web interface |
| `opcache` | true | Enable PHP OPcache |
| `ssl` | false | Enable HTTPS (requires certificates) |

## Usage Examples

### Basic Development Setup

```yaml
include:
  - blueprint: blueprints/kscore/lamp-stack@1.0.0
    params:
      server_name: localhost
      mysql_root_password: devpassword
      mysql_password: devpassword
```

### Production Setup

```yaml
include:
  - blueprint: blueprints/kscore/lamp-stack@1.0.0
    params:
      server_name: www.example.com
      server_admin: ops@example.com
      document_root: /var/www/example.com/public
      php_memory_limit: 512M
      php_max_execution_time: 60
      mysql_max_connections: 200
      mysql_root_password: !secret databases/mysql/root
      mysql_password: !secret databases/mysql/app
    features:
      opcache: true
      ssl: true
```

### Multiple Instances

```yaml
# Development site
include:
  - blueprint: blueprints/kscore/lamp-stack@1.0.0
    as: dev-site
    params:
      server_name: dev.example.com
      document_root: /var/www/dev.example.com
      mysql_database: dev_db
      mysql_user: dev_user
      mysql_root_password: !secret mysql/root
      mysql_password: !secret mysql/dev

# Staging site
  - blueprint: blueprints/kscore/lamp-stack@1.0.0
    as: staging-site
    params:
      server_name: staging.example.com
      document_root: /var/www/staging.example.com
      mysql_database: staging_db
      mysql_user: staging_user
      mysql_root_password: !secret mysql/root
      mysql_password: !secret mysql/staging
```

## Platform Support

| Platform | Support Level |
|----------|---------------|
| Debian 11/12 | Full |
| Ubuntu 20.04/22.04/24.04 | Full |
| RHEL 8/9 | Full |
| CentOS Stream 8/9 | Full |
| Rocky Linux 8/9 | Full |
| AlmaLinux 8/9 | Full |

## Security Considerations

1. **Always use secrets** for passwords - never hardcode credentials
2. **MySQL bind address** defaults to 127.0.0.1 for security
3. **Security headers** are configured by default (X-Content-Type-Options, X-Frame-Options, etc.)
4. **Sensitive files** are blocked by Apache configuration
5. **PHP expose_php** is disabled by default

## Testing

Run blueprint tests:

```bash
kscorectl blueprint test blueprints/kscore/lamp-stack
```

## Customization

### Adding PHP Extensions

Create a state file that extends the blueprint:

```yaml
include:
  - blueprint: blueprints/kscore/lamp-stack@1.0.0
    params:
      server_name: example.com
      # ... other params

# Additional PHP extensions
extra_php_extensions:
  pkg.installed:
    - pkgs:
      - php-imagick
      - php-redis
    - require:
      - pkg: php_packages
```

### Custom Apache Configuration

Place additional configuration files in your state:

```yaml
include:
  - blueprint: blueprints/kscore/lamp-stack@1.0.0
    params:
      # ... params

# Custom Apache config
custom_apache_config:
  file.managed:
    - name: /etc/apache2/conf-available/custom.conf
    - contents: |
        # Custom configuration
        ServerTokens Prod
        ServerSignature Off
    - require:
      - pkg: apache_package
```

## Troubleshooting

### Apache won't start

Check for syntax errors:
```bash
apachectl configtest
```

### PHP errors

Check PHP configuration:
```bash
php -i | grep -E "(error_log|display_errors)"
```

### MySQL connection issues

Verify MySQL is running and accessible:
```bash
systemctl status mariadb
mysql -u root -p -e "SELECT 1"
```

## Version History

- **1.0.0** - Initial release with Apache, PHP 8.2, MariaDB

## License

Apache License 2.0 - See LICENSE file for details.
