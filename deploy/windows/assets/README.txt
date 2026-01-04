Keystone Core Agent for Windows
================================

Thank you for installing the Keystone Core Agent!

Configuration
-------------
The agent configuration file is located at:
  C:\ProgramData\kscore\agent.yaml

Edit this file to configure your control plane connection.

Service Management
------------------
The agent runs as a Windows service named "KeystoneCoreAgent".

To manage the service:
  Start:   net start KeystoneCoreAgent
           sc start KeystoneCoreAgent

  Stop:    net stop KeystoneCoreAgent
           sc stop KeystoneCoreAgent

  Status:  sc query KeystoneCoreAgent

Or use PowerShell:
  Get-Service KeystoneCoreAgent
  Start-Service KeystoneCoreAgent
  Stop-Service KeystoneCoreAgent

Event Logs
----------
Agent logs are written to the Windows Event Log.
Open Event Viewer and look under:
  Windows Logs > Application
  Filter by Source: KeystoneCore

Command Line Tools
------------------
The following tools are installed and added to PATH:
  kscorectl    - Main CLI tool
  kscore-exec  - Remote execution commands
  kscore-state - State management commands

Example:
  kscorectl agent status
  kscore-exec run "hostname" --target "this"

Documentation
-------------
Full documentation available at:
  https://github.com/shawnbutts/keystone-core

Support
-------
Report issues at:
  https://github.com/shawnbutts/keystone-core/issues

License
-------
Apache License 2.0
See LICENSE.txt for details.
