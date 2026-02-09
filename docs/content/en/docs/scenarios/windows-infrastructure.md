---
title: "Windows Infrastructure"
weight: 26
description: >
  Manage Windows servers with Active Directory integration and PowerShell automation
---

This scenario demonstrates managing Windows infrastructure with Keystone Core, including Active Directory, Group Policy, and PowerShell-based automation.

> **Implementation Note**: This scenario includes both implemented and planned features:
> - **Implemented**: `win_service`, `win_registry`, `win_firewall`, `win_package`, `win_feature`, `scheduled_task`, `cmd` (PowerShell execution)
> - **Planned**: `ad_user`, `ad_group`, `gpo`, `dsc` modules are described below as conceptual designs. Until implemented, use the `cmd` module with PowerShell scripts to manage Active Directory, Group Policy, and DSC configurations.

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

> **Planned Feature**: The `ad_user` module is planned but not yet implemented.
> Use the `cmd` module with PowerShell for AD management until then.

**Planned API** (conceptual design):
```yaml
# ad-users.yaml - Planned ad_user module API
metadata:
  name: ad-users

target: "role:domain-controller"

variables:
  users: []  # List of users to manage

ad_user:
{{- range $user := .vars.users }}
  user_{{ $user.username }}:
    state: present
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

**Current workaround** using PowerShell:
```yaml
# ad-users-workaround.yaml
metadata:
  name: ad-users

target: "role:domain-controller"

cmd:
{{- range $user := .vars.users }}
  create_user_{{ $user.username }}:
    state: run
    command: |
      Import-Module ActiveDirectory
      $user = Get-ADUser -Filter "SamAccountName -eq '{{ $user.username }}'" -ErrorAction SilentlyContinue
      if (-not $user) {
        New-ADUser -Name "{{ $user.username }}" -DisplayName "{{ $user.display_name }}" `
          -EmailAddress "{{ $user.email }}" -Department "{{ $user.department }}" -Enabled $true
      }
    shell: powershell
{{- end }}
```

### 2. Group Policy Management

> **Planned Feature**: The `gpo` module is planned but not yet implemented.
> Use the `cmd` module with PowerShell GroupPolicy cmdlets until then.

**Planned API** (conceptual design):
```yaml
# gpo-security-baseline.yaml - Planned gpo module API
metadata:
  name: gpo-security-baseline

target: "role:domain-controller"

variables:
  domain: "example.com"
  domain_components: "example,DC=com"

gpo:
  security_baseline:
    state: present
    name: "Keystone Security Baseline"
    domain: "{{ .vars.domain }}"
    link_enabled: true
    links:
      - ou: "OU=Servers,DC={{ .vars.domain_components }}"
        enforced: true
    settings:
      computer_configuration:
        policies:
          windows_settings:
            security_settings:
              account_policies:
                password_policy:
                  minimum_password_length: 14
                  password_complexity: enabled
                  maximum_password_age: 90
```

**Current workaround** using PowerShell:
```yaml
# gpo-workaround.yaml
cmd:
  create_gpo:
    state: run
    command: |
      Import-Module GroupPolicy
      $gpo = Get-GPO -Name "Keystone Security Baseline" -ErrorAction SilentlyContinue
      if (-not $gpo) {
        New-GPO -Name "Keystone Security Baseline" -Comment "Managed by Keystone Core"
      }
    shell: powershell
```

### 3. Windows Service Management

```yaml
# windows-services.yaml
metadata:
  name: windows-services

target: "os:Windows"

win_service:
  # Disable unnecessary services
  disable_remote_registry:
    state: stopped
    name: RemoteRegistry
    startup_type: disabled

  disable_xbox_services:
    state: stopped
    name: XboxGipSvc
    startup_type: disabled

  # Ensure critical services are running
  windows_defender:
    state: running
    name: WinDefend
    startup_type: automatic

  windows_firewall:
    state: running
    name: mpssvc
    startup_type: automatic
```

### 4. Windows Firewall Rules

```yaml
# windows-firewall.yaml
metadata:
  name: windows-firewall

target: "os:Windows"

variables:
  management_network: "10.0.0.0/8"

win_firewall:
  allow_rdp_internal:
    state: present
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

  allow_winrm:
    state: present
    name: "Allow WinRM HTTPS"
    direction: inbound
    action: allow
    protocol: tcp
    local_port: 5986
    remote_addresses:
      - "{{ .vars.management_network }}"
    enabled: true

  block_smb_external:
    state: present
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

> **Planned Feature**: The `dsc` module is planned but not yet implemented.
> Use the `cmd` module with PowerShell to invoke DSC configurations until then.

**Planned API** (conceptual design):
```yaml
# dsc-configuration.yaml - Planned dsc module API
metadata:
  name: dsc-configuration

target: "os:Windows"

dsc:
  web_server_config:
    state: present
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
          }
      }
    configuration_name: WebServerConfig
    refresh_mode: push
```

**Current workaround** using PowerShell and win_feature:
```yaml
# dsc-workaround.yaml
win_feature:
  iis:
    state: present
    name: Web-Server

  iis_management:
    state: present
    name: Web-Mgmt-Tools
    require:
      - win_feature: iis

file:
  web_content_dir:
    state: directory
    name: 'C:\inetpub\wwwroot\app'
    require:
      - win_feature: iis
```

### 6. Windows Registry Management

```yaml
# windows-registry.yaml
metadata:
  name: windows-registry

target: "os:Windows"

win_registry:
  # Security hardening
  disable_autorun:
    state: present
    key: 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\Explorer'
    value_name: NoDriveTypeAutoRun
    value_data: 255
    value_type: dword

  enable_uac:
    state: present
    key: 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System'
    value_name: EnableLUA
    value_data: 1
    value_type: dword

  disable_wdigest:
    state: present
    key: 'HKLM:\SYSTEM\CurrentControlSet\Control\SecurityProviders\WDigest'
    value_name: UseLogonCredential
    value_data: 0
    value_type: dword
```

## Verification

```bash
# Check Windows agent status
kscorectl agents list --label "os=Windows"

# Verify AD users
kscorectl exec run "role:domain-controller" -- \
  powershell -Command "Get-ADUser -Filter * | Select Name, Enabled"

# Check Group Policy application
kscorectl exec run "os:Windows" -- \
  powershell -Command "gpresult /r"

# Verify services
kscorectl exec run "os:Windows" -- \
  powershell -Command "Get-Service WinDefend, mpssvc | Format-Table"
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
