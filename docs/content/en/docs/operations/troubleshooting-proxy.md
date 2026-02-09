---
title: "Proxy Agent Troubleshooting"
weight: 18
description: >
  Troubleshooting guide for proxy agent connections, state modules, NETCONF, and vendor-specific issues
---

## Connection Issues

### Device Not Reachable

**Symptoms**: Connection timeout, "no route to host", or "connection refused" errors.

**Steps**:

1. Verify network connectivity from the proxy agent host:

    ```bash
    kscorectl proxy device ping <device-id>
    kscorectl proxy device test <device-id> --verbose
    ```

2. Check that the device is listening on the expected port:

    ```bash
    kscorectl proxy device status <device-id>
    ```

3. Verify firewall rules allow traffic from the proxy agent to the device on the management port (SSH: 22, SNMP: 161, NETCONF: 830, REST: 443).

4. For NETCONF devices, ensure the NETCONF subsystem is enabled:

    - **Cisco IOS-XE**: `netconf-yang` must be enabled in global config
    - **Juniper JUNOS**: `set system services netconf ssh` in config
    - **Arista EOS**: `management api netconf-yang` must be enabled

### Authentication Failures

**Symptoms**: "authentication failed", "access denied", or "invalid credentials" errors.

**Steps**:

1. Verify the credential exists and is valid:

    ```bash
    kscorectl proxy credential verify <credential-ref>
    ```

2. Test the connection with the credential:

    ```bash
    kscorectl proxy device connect <device-id> --credential <credential-ref>
    ```

3. Check that the user account has sufficient privileges on the device (admin/super_admin for REST API modules, privilege level 15 for Cisco IOS).

4. For SSH key authentication, verify the public key is installed on the device and the private key file has correct permissions (mode 0600).

5. For REST API authentication, verify the API key or token has not expired.

### Connection Timeouts

**Symptoms**: Operations hang or fail with timeout errors.

**Steps**:

1. Increase the connection timeout in device configuration:

    ```yaml
    proxy:
      devices:
        - id: slow-device
          timeout: 60s
    ```

2. Check for network latency or packet loss between the proxy agent and the device.

3. For SSH connections, verify the device is not running out of SSH session slots.

4. For REST API connections, check if the device is under heavy load or rate-limiting requests.

## State Module Issues

### Module Not Found

**Symptoms**: "unknown module" error when applying state.

**Steps**:

1. Verify the module name is correct. Common modules:
   - SSH: `ssh_file`, `ssh_cmd`, `ssh_service`, `ssh_package`, `ssh_user`, `ssh_group`
   - SNMP: `snmp_value`, `snmp_table`
   - REST: `http_config`, `http_resource`
   - WinRM: `winrm_file`, `winrm_service`, `winrm_registry`, `winrm_package`
   - NETCONF: `netconf_interface`, `netconf_vlan`, `netconf_routing`, `netconf_acl`
   - Vendor: `fortios_policy`, `panos_rule`, `bigip_pool`, `bigip_virtual`, `checkpoint_rule`
   - Network config: `ios_config`, `nxos_config`, `junos_config`, `eos_config`, etc.

2. Check the [Proxy State Modules Reference]({{< relref "../reference/proxy-state-modules.md" >}}) for the complete list.

### Required Parameter Missing

**Symptoms**: "required parameter missing" error with the parameter name.

**Steps**:

1. Check the module's parameter table in the [Proxy State Modules Reference]({{< relref "../reference/proxy-state-modules.md" >}}).

2. Ensure the parameter is provided in the `parameters` map of your state definition:

    ```yaml
    proxy_state:
      - module: netconf_interface
        parameters:
          name: GigabitEthernet0/0/0  # required
          description: "Uplink"       # optional
    ```

### Dry-Run Shows Changes But Apply Fails

**Symptoms**: Dry-run reports `changed: true` but applying the state fails with an error.

**Steps**:

1. Check the error message for device-specific details (permission denied, invalid value, etc.).

2. Verify the device supports the operation. For example, not all NETCONF devices support all OpenConfig models.

3. For NETCONF modules, check that the device supports the candidate datastore:

    ```bash
    kscorectl proxy exec <device-id> --protocol netconf "get-config running"
    ```

4. For vendor modules (FortiOS, PAN-OS, BIG-IP, Check Point), verify the API is accessible and the credentials have write access.

## NETCONF Issues

### Lock Denied

**Symptoms**: "lock-denied" error when applying NETCONF state.

**Cause**: Another session holds a lock on the candidate datastore.

**Steps**:

1. Wait for the other session to complete and release the lock.

2. If the lock is stale, kill the blocking session:

    ```bash
    kscorectl proxy exec <device-id> --protocol netconf "kill-session <session-id>"
    ```

3. Check for other management tools (e.g., YANG Suite, NSO) that may hold locks.

### Edit-Config Validation Failure

**Symptoms**: "invalid-value" or validation error after edit-config.

**Steps**:

1. Verify the XML configuration is well-formed and uses the correct OpenConfig namespace.

2. Check that the device supports the specific OpenConfig model version:
   - `openconfig-interfaces` for interface management
   - `openconfig-vlan` for VLAN management
   - `openconfig-network-instance` for routing/VRF
   - `openconfig-acl` for access control lists

3. Some devices require vendor-specific augmentations. Check the device documentation for supported YANG models.

4. The NETCONF modules automatically discard changes and unlock the candidate datastore on failure.

### Commit Failure

**Symptoms**: Commit operation fails after successful edit-config.

**Steps**:

1. Check the error message for conflicting configuration or dependency issues.

2. Verify no other pending changes exist in the candidate datastore:

    ```bash
    kscorectl proxy exec <device-id> --protocol netconf "discard-changes"
    ```

3. For devices with confirmed-commit support, use a confirm timeout to allow automatic rollback on failure.

## Vendor-Specific Issues

### FortiGate (fortios_policy)

**API connection issues**:

1. Verify the REST API is enabled on the FortiGate.
2. Check that the API user has the `super_admin` access profile.
3. Verify the trust host configuration allows connections from the proxy agent IP.
4. Check for CSRF token requirements (FortiOS 6.2+ may require CSRF handling).

**Policy not applied**:

1. Verify the `policy_id` does not conflict with an existing policy.
2. Check that referenced address objects (`srcaddr`, `dstaddr`) and service objects exist on the FortiGate.
3. Verify interface names match the device configuration exactly.

### Palo Alto (panos_rule)

**XML API errors**:

1. Verify the API key is valid and has not expired.
2. Check that the user has sufficient admin role permissions.
3. For commit failures, check the commit log on the device for conflicting changes.

**Rule not visible**:

1. If `commit: true` is not set, changes are only in the candidate configuration. Set `commit: true` or commit manually.
2. Verify the rule is in the correct device group for Panorama-managed devices.

### F5 BIG-IP (bigip_pool, bigip_virtual)

**iControl REST errors**:

1. Verify iControl REST is accessible on port 443.
2. Check that the user has the `Administrator` role.
3. For pool member issues, verify member addresses are reachable from the BIG-IP.

**Virtual server not working**:

1. Verify the pool exists before creating a virtual server that references it.
2. Check that profiles (`http`, `clientssl`) exist on the BIG-IP.
3. For SNAT issues, verify SNAT pools or automap configuration.

### Check Point (checkpoint_rule)

**Web API session issues**:

1. Verify the Web Services API is enabled on the management server.
2. Check that API access is allowed from the proxy agent IP.
3. Session management is handled automatically (login/publish/logout per operation).

**Rule not published**:

1. The module automatically publishes changes after successful rule creation/modification.
2. If publish fails, check for conflicting changes by other administrators.
3. Verify the policy layer name matches an existing layer (default: "Network").

## Performance Issues

### Slow State Application

1. Reduce the number of concurrent device operations if the proxy agent is overloaded.
2. For NETCONF operations, the lock/edit/validate/commit/unlock workflow adds latency — this is expected for transactional safety.
3. For REST API modules, check if the device is rate-limiting requests.

### High Memory Usage

1. Check the number of managed devices — each active connection consumes memory.
2. Reduce connection pool sizes for devices accessed infrequently.
3. Monitor with proxy agent metrics:

    ```bash
    curl http://proxy-agent:9090/metrics | grep kscore_proxy
    ```

## Diagnostic Commands

```bash
# Overall proxy agent status
kscorectl proxy status

# Device connectivity test
kscorectl proxy device test <device-id> --verbose

# Credential verification
kscorectl proxy credential verify <credential-ref>

# State dry-run
kscorectl proxy state apply --device <device-id> --dry-run

# State execution logs
kscorectl proxy state logs --device <device-id> --run-id <run-id>

# Device configuration dump
kscorectl proxy device config show <device-id>

# NETCONF capability check
kscorectl proxy exec <device-id> --protocol netconf "get-config running"
```

## See Also

- [Proxy Agents]({{< relref "../concepts/proxy-agents.md" >}})
- [Proxy State Modules Reference]({{< relref "../reference/proxy-state-modules.md" >}})
- [Vendor Configuration Guide]({{< relref "vendor-configuration.md" >}})
- [Protocol Compatibility Matrix]({{< relref "../reference/compatibility-matrix.md" >}})
- [NETCONF Protocol Reference]({{< relref "../reference/netconf.md" >}})
- [Vendor Drivers Reference]({{< relref "../reference/vendor-drivers.md" >}})
