---
title: "Windows Infrastructure"
weight: 26
description: >
  Manage Windows servers with Active Directory integration and PowerShell automation
---

This scenario demonstrates managing Windows infrastructure with Keystone Core, including Active Directory, Group Policy, and PowerShell-based automation.

## Overview

Windows environments require specialized management approaches:

- **Active Directory Integration**: User, group, and computer management
- **Group Policy**: Centralized configuration management
- **PowerShell**: Native scripting and automation
- **Windows Services**: Application lifecycle management

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Windows Domain Environment                    │
│                                                                  │
│  ┌─────────────────┐      ┌─────────────────────────────────┐  │
│  │ Domain          │      │         Member Servers           │  │
│  │ Controllers     │      │  ┌─────┐ ┌─────┐ ┌─────┐       │  │
│  │ ┌────┐ ┌────┐  │      │  │ Web │ │ App │ │ DB  │       │  │
│  │ │DC1 │ │DC2 │  │◄────►│  │     │ │     │ │     │       │  │
│  │ └────┘ └────┘  │      │  └──┬──┘ └──┬──┘ └──┬──┘       │  │
│  └────────┬────────┘      │     │       │       │          │  │
│           │               │     └───────┴───────┘          │  │
│           │               │           Agents                │  │
│           │               └─────────────────────────────────┘  │
│           │                                                     │
│  ┌────────┴────────┐                                           │
│  │ Keystone Core   │                                           │
│  │ Control Plane   │                                           │
│  └─────────────────┘                                           │
└─────────────────────────────────────────────────────────────────┘
```

## Prerequisites

- Windows Server 2019/2022
- Active Directory domain
- Keystone Core agent installed via MSI
- PowerShell 5.1+ or PowerShell 7

## Implementation

### 1. Active Directory User Management

```yaml
# ad-users.yaml
apiVersion: v1
kind: state
metadata:
  name: ad-users

target: "role:domain-controller"

parameters:
  users:
    type: list
    description: "List of users to manage"

resources:
  {{- range $user := parameters.users }}
  - type: windows.ad_user
    name: "user-{{ $user.username }}"
    properties:
      name: "{{ $user.username }}"
      display_name: "{{ $user.display_name }}"
      email: "{{ $user.email }}"
      department: "{{ $user.department }}"
      enabled: true
      password_never_expires: false
      must_change_password: {{ $user.new_user | default false }}
      groups:
        - "{{ $user.department }}-Users"
        {{- if $user.admin }}
        - "Domain Admins"
        {{- end }}
  {{- end }}
```

### 2. Group Policy Management

```yaml
# gpo-security-baseline.yaml
apiVersion: v1
kind: state
metadata:
  name: gpo-security-baseline

target: "role:domain-controller"

resources:
  - type: windows.gpo
    name: security-baseline
    properties:
      name: "Keystone Security Baseline"
      domain: "{{ pillar.domain }}"
      link_enabled: true
      links:
        - ou: "OU=Servers,DC={{ pillar.domain_components }}"
          enforced: true
      settings:
        # Password policy
        computer_configuration:
          policies:
            windows_settings:
              security_settings:
                account_policies:
                  password_policy:
                    minimum_password_length: 14
                    password_complexity: enabled
                    maximum_password_age: 90
                    minimum_password_age: 1
                    password_history: 24
                  account_lockout_policy:
                    lockout_threshold: 5
                    lockout_duration: 30
                    reset_counter: 30
                local_policies:
                  audit_policy:
                    audit_logon_events: "Success, Failure"
                    audit_account_management: "Success, Failure"
                    audit_policy_change: "Success, Failure"
```

### 3. Windows Service Management

```yaml
# windows-services.yaml
apiVersion: v1
kind: state
metadata:
  name: windows-services

target: "os:Windows"

resources:
  # Disable unnecessary services
  - type: windows.service
    name: disable-remote-registry
    properties:
      name: RemoteRegistry
      startup_type: disabled
      state: stopped

  - type: windows.service
    name: disable-xbox-services
    properties:
      name: XboxGipSvc
      startup_type: disabled
      state: stopped

  # Ensure critical services are running
  - type: windows.service
    name: windows-defender
    properties:
      name: WinDefend
      startup_type: automatic
      state: running

  - type: windows.service
    name: windows-firewall
    properties:
      name: mpssvc
      startup_type: automatic
      state: running
```

### 4. Windows Firewall Rules

```yaml
# windows-firewall.yaml
apiVersion: v1
kind: state
metadata:
  name: windows-firewall

target: "os:Windows"

resources:
  - type: windows.firewall_rule
    name: allow-rdp-internal
    properties:
      name: "Allow RDP from Internal"
      direction: inbound
      action: allow
      protocol: tcp
      local_port: 3389
      remote_addresses:
        - "10.0.0.0/8"
        - "192.168.0.0/16"
      profiles:
        - domain
        - private
      enabled: true

  - type: windows.firewall_rule
    name: allow-winrm
    properties:
      name: "Allow WinRM HTTPS"
      direction: inbound
      action: allow
      protocol: tcp
      local_port: 5986
      remote_addresses:
        - "{{ pillar.management_network }}"
      enabled: true

  - type: windows.firewall_rule
    name: block-smb-external
    properties:
      name: "Block SMB from External"
      direction: inbound
      action: block
      protocol: tcp
      local_port: 445
      profiles:
        - public
      enabled: true
```

### 5. PowerShell DSC Integration

```yaml
# dsc-configuration.yaml
apiVersion: v1
kind: state
metadata:
  name: dsc-configuration

target: "os:Windows"

resources:
  - type: windows.dsc
    name: web-server-config
    properties:
      configuration: |
        Configuration WebServerConfig {
            Import-DscResource -ModuleName PSDesiredStateConfiguration
            Import-DscResource -ModuleName WebAdministration

            Node localhost {
                WindowsFeature IIS {
                    Ensure = 'Present'
                    Name   = 'Web-Server'
                }

                WindowsFeature IISManagement {
                    Ensure    = 'Present'
                    Name      = 'Web-Mgmt-Tools'
                    DependsOn = '[WindowsFeature]IIS'
                }

                File WebContent {
                    Ensure          = 'Present'
                    Type            = 'Directory'
                    DestinationPath = 'C:\inetpub\wwwroot\app'
                    DependsOn       = '[WindowsFeature]IIS'
                }
            }
        }
      configuration_name: WebServerConfig
      refresh_mode: push
```

### 6. Windows Registry Management

```yaml
# windows-registry.yaml
apiVersion: v1
kind: state
metadata:
  name: windows-registry

target: "os:Windows"

resources:
  # Security hardening
  - type: windows.registry
    name: disable-autorun
    properties:
      key: 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\Explorer'
      value_name: NoDriveTypeAutoRun
      value_data: 255
      value_type: dword

  - type: windows.registry
    name: enable-uac
    properties:
      key: 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System'
      value_name: EnableLUA
      value_data: 1
      value_type: dword

  - type: windows.registry
    name: disable-wdigest
    properties:
      key: 'HKLM:\SYSTEM\CurrentControlSet\Control\SecurityProviders\WDigest'
      value_name: UseLogonCredential
      value_data: 0
      value_type: dword
```

## Verification

```bash
# Check Windows agent status
kscorectl ping -t "os:Windows"

# Verify AD users
kscorectl exec -t "role:domain-controller" -s powershell -- \
  "Get-ADUser -Filter * | Select Name, Enabled"

# Check Group Policy application
kscorectl exec -t "os:Windows" -s powershell -- \
  "gpresult /r"

# Verify services
kscorectl exec -t "os:Windows" -s powershell -- \
  "Get-Service WinDefend, mpssvc | Format-Table"
```

## Troubleshooting

### Agent Communication Issues

Check WinRM configuration:
```powershell
winrm quickconfig
winrm get winrm/config
```

### GPO Not Applying

Force Group Policy refresh:
```powershell
gpupdate /force
```

### DSC Configuration Drift

Check DSC status:
```powershell
Get-DscConfigurationStatus
Test-DscConfiguration -Detailed
```

## Next Steps

- [Compliance Automation]({{< relref "compliance-automation" >}}) - CIS benchmarks for Windows
- [Disaster Recovery]({{< relref "disaster-recovery" >}}) - Windows backup strategies
