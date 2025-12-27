#!/bin/bash
set -e

# Create kscore user and group if they don't exist
if ! getent group kscore >/dev/null 2>&1; then
    groupadd --system kscore
fi

if ! getent passwd kscore >/dev/null 2>&1; then
    useradd --system --gid kscore --home-dir /var/lib/keystone-core \
        --shell /sbin/nologin --comment "Keystone Core" kscore
fi

# Set ownership of directories
chown -R kscore:kscore /var/lib/keystone-core
chown -R kscore:kscore /var/log/keystone-core
chmod 750 /var/lib/keystone-core
chmod 750 /var/log/keystone-core

# Set ownership of config directory
if [ -d /etc/keystone-core ]; then
    chown root:kscore /etc/keystone-core
    chmod 750 /etc/keystone-core
    # Config files should be readable by kscore group
    find /etc/keystone-core -type f -exec chmod 640 {} \;
    find /etc/keystone-core -type f -exec chown root:kscore {} \;
fi

# Reload systemd
systemctl daemon-reload

# Enable service but don't start automatically
# User should configure before starting
echo ""
echo "Keystone Core Server installed successfully!"
echo ""
echo "Next steps:"
echo "  1. Edit /etc/keystone-core/server.yaml to configure the server"
echo "  2. Start the service: systemctl start kscore-server"
echo "  3. Enable on boot: systemctl enable kscore-server"
echo ""
echo "Documentation: https://docs.kscore.dev"
echo ""
