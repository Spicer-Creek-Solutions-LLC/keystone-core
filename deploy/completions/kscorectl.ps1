# PowerShell completion for kscorectl and kscore-* plugins
# Add to your PowerShell profile: . /path/to/kscorectl.ps1

using namespace System.Management.Automation
using namespace System.Management.Automation.Language

Register-ArgumentCompleter -Native -CommandName kscorectl -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $commands = @{
        'exec' = 'Execute commands on remote agents'
        'state' = 'Manage declarative state configurations'
        'module' = 'Manage Keystone Core modules'
        'policy' = 'Policy enforcement and compliance'
        'gitops' = 'GitOps integration and deployment'
        'blueprint' = 'Manage reusable infrastructure patterns'
        'cluster' = 'Manage Keystone Core cluster'
        'identity' = 'SPIFFE identity and certificate management'
        'files' = 'File distribution server'
        'audit' = 'Policy audit and compliance reporting'
        'webhook' = 'Webhook handler management'
        'federation' = 'Trust federation management'
        'bootstrap' = 'Cluster bootstrap and initialization'
        'monitor' = 'Real-time TUI monitoring dashboard'
        'migrate' = 'Database migration utilities'
        'version' = 'Show version information'
        'help' = 'Show help information'
    }

    $subcommands = @{
        'exec' = @{
            'run' = 'Execute command across matching agents'
            'status' = 'Get batch job status'
            'list' = 'List batch jobs'
            'version' = 'Print version information'
        }
        'state' = @{
            'apply' = 'Apply state declarations'
            'check' = 'Check state without applying (dry-run)'
            'drift' = 'Detect drift from desired state'
            'version' = 'Print version information'
        }
        'module' = @{
            'init' = 'Create new module from template'
            'validate' = 'Check module.yaml syntax'
            'build' = 'Package module as ZIP'
            'resolve' = 'Resolve dependencies'
            'tree' = 'Display dependency tree'
            'verify' = 'Verify cryptographic signatures'
            'sign' = 'Sign module with private key'
            'test' = 'Run module tests'
            'publish' = 'Publish to registry'
            'install' = 'Install from registry'
            'version' = 'Print version information'
        }
        'policy' = @{
            'list' = 'List policies from file'
            'validate' = 'Validate policy syntax'
            'check' = 'Evaluate policy against input'
            'show' = 'Show policy details'
            'audit' = 'Show audit log (deprecated)'
            'report' = 'Generate compliance report (deprecated)'
            'version' = 'Print version information'
        }
        'gitops' = @{
            'verify' = 'Run verification workflow'
            'rollback' = 'Trigger rollback operation'
            'promote' = 'Promote between environments'
            'status' = 'Show GitOps operation status'
            'webhook' = 'Webhook commands (deprecated)'
            'version' = 'Print version information'
        }
        'blueprint' = @{
            'init' = 'Create new blueprint'
            'validate' = 'Check blueprint syntax'
            'lint' = 'Run best practices checks'
            'test' = 'Run blueprint tests'
            'search' = 'Search for blueprints'
            'info' = 'Show blueprint details'
            'versions' = 'List available versions'
            'install' = 'Install blueprint'
            'update' = 'Update blueprints'
            'remove' = 'Remove blueprint'
            'publish' = 'Publish blueprint (deprecated)'
            'sign' = 'Sign blueprint (deprecated)'
            'verify' = 'Verify signature (deprecated)'
            'version' = 'Print version information'
        }
        'cluster' = @{
            'status' = 'View cluster status'
            'members' = 'Manage cluster members'
            'leader' = 'Monitor leader election'
            'add' = 'Add cluster member'
            'remove' = 'Remove cluster member'
            'transfer-leader' = 'Transfer leader role'
            'rebalance' = 'Rebalance cluster'
            'backup' = 'Backup cluster (deprecated)'
            'restore' = 'Restore from backup (deprecated)'
            'version' = 'Print version information'
        }
        'identity' = @{
            'status' = 'Show identity provider status'
            'token' = 'Manage join tokens'
            'ca' = 'Certificate authority management'
            'federation' = 'Federation management'
            'bundle' = 'Trust bundle management'
            'events' = 'View identity events'
            'version' = 'Print version information'
        }
        'files' = @{
            'serve' = 'Start the file server'
            'files' = 'File management'
            'cache' = 'Cache management'
            'namespace' = 'Namespace management'
            'backend' = 'Backend configuration (deprecated)'
            'mirrors' = 'Mirror configuration (deprecated)'
            'version' = 'Print version information'
        }
        'audit' = @{
            'log' = 'View policy evaluation audit log'
            'report' = 'Generate compliance report'
            'export' = 'Export audit data'
            'stats' = 'View audit statistics and trends'
            'version' = 'Print version information'
        }
        'webhook' = @{
            'list' = 'List registered webhook handlers'
            'show' = 'Show webhook handler details'
            'test' = 'Send test webhook'
            'history' = 'View webhook delivery history'
            'secrets' = 'Manage webhook secrets'
            'version' = 'Print version information'
        }
        'federation' = @{
            'list' = 'List federated trust domains'
            'add' = 'Add federated domain'
            'show' = 'Show federation details'
            'suspend' = 'Suspend federation'
            'activate' = 'Activate federation'
            'remove' = 'Remove federation'
            'refresh' = 'Refresh trust bundle'
            'bundle' = 'Manage trust bundles'
            'version' = 'Print version information'
        }
        'bootstrap' = @{
            'seed' = 'Bootstrap from seed configuration'
            'restore' = 'Restore from backup'
            'import' = 'Import existing installation'
            'validate' = 'Validate configuration'
            'status' = 'Show bootstrap status'
            'cleanup' = 'Clean up bootstrap resources'
            'version' = 'Print version information'
        }
    }

    $nestedSubcommands = @{
        'identity:token' = @{
            'create' = 'Create join token'
            'list' = 'List join tokens'
            'show' = 'Show token details'
            'revoke' = 'Revoke token'
        }
        'identity:ca' = @{
            'info' = 'Show CA information'
            'backup' = 'Backup CA certificates'
            'restore' = 'Restore CA from backup'
            'rotate' = 'Trigger CA rotation'
        }
        'identity:bundle' = @{
            'show' = 'Show local trust bundle'
            'export' = 'Export trust bundle'
        }
        'webhook:secrets' = @{
            'list' = 'List webhook secrets'
            'rotate' = 'Rotate webhook secret'
        }
    }

    $flagValues = @{
        '--log-level' = @('debug', 'info', 'warn', 'error')
        '--audit-level' = @('debug', 'info', 'warn', 'error', 'all')
        '--audit-output' = @('auto', 'file', 'syslog')
        '--output' = @('table', 'text', 'json', 'yaml')
        '-o' = @('table', 'text', 'json', 'yaml')
        '--format' = @('table', 'text', 'json', 'yaml')
        '--type' = @('opa', 'cel')
        '--status' = @('pending', 'running', 'completed', 'failed')
        '--strategy' = @('previous', 'specific', 'last_known_good')
        '--severity' = @('low', 'medium', 'high', 'critical')
        '--export-format' = @('json', 'csv', 'yaml')
        '--theme' = @('dark', 'light', 'solarized-dark', 'solarized-light', 'monokai')
    }

    # Parse command line
    $elements = $commandAst.CommandElements
    $cmd = $null
    $subcmd = $null
    $nestedcmd = $null

    for ($i = 1; $i -lt $elements.Count; $i++) {
        $element = $elements[$i].ToString()
        if ($element -notlike '-*') {
            if ($null -eq $cmd) {
                $cmd = $element
            } elseif ($null -eq $subcmd) {
                $subcmd = $element
            } elseif ($null -eq $nestedcmd) {
                $nestedcmd = $element
            }
        }
    }

    # Check if completing a flag value
    $lastElement = $elements[-1].ToString()
    if ($flagValues.ContainsKey($lastElement)) {
        $flagValues[$lastElement] | ForEach-Object {
            [CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    # Top-level command completion
    if ($null -eq $cmd -or ($elements.Count -eq 2 -and $wordToComplete)) {
        $commands.GetEnumerator() | Where-Object { $_.Key -like "$wordToComplete*" } | ForEach-Object {
            [CompletionResult]::new($_.Key, $_.Key, 'Command', $_.Value)
        }
        return
    }

    # Nested subcommand completion (e.g., identity token create)
    $nestedKey = "$cmd`:$subcmd"
    if ($nestedSubcommands.ContainsKey($nestedKey) -and ($null -eq $nestedcmd -or $wordToComplete)) {
        $nestedSubcommands[$nestedKey].GetEnumerator() | Where-Object { $_.Key -like "$wordToComplete*" } | ForEach-Object {
            [CompletionResult]::new($_.Key, $_.Key, 'Command', $_.Value)
        }
        return
    }

    # Subcommand completion
    if ($subcommands.ContainsKey($cmd) -and ($null -eq $subcmd -or ($elements.Count -eq 3 -and $wordToComplete))) {
        $subcommands[$cmd].GetEnumerator() | Where-Object { $_.Key -like "$wordToComplete*" } | ForEach-Object {
            [CompletionResult]::new($_.Key, $_.Key, 'Command', $_.Value)
        }
        return
    }

    # Flag completion
    if ($wordToComplete -like '-*') {
        $globalFlags = @(
            @{Name='--config'; Desc='Path to configuration file'}
            @{Name='--log-level'; Desc='Set log level'}
            @{Name='--audit-level'; Desc='Audit logging level'}
            @{Name='--audit-output'; Desc='Audit output backend'}
            @{Name='--help'; Desc='Show help'}
        )

        $cmdFlags = @{
            'exec' = @(
                @{Name='--server'; Desc='Keystone Core server address'}
                @{Name='--timeout'; Desc='Request timeout'}
                @{Name='--tls'; Desc='Enable TLS'}
                @{Name='--tls-ca-cert'; Desc='Path to CA certificate'}
                @{Name='--tls-cert'; Desc='Path to client certificate'}
                @{Name='--tls-key'; Desc='Path to client private key'}
                @{Name='--tls-skip-verify'; Desc='Skip TLS verification'}
                @{Name='--target'; Desc='Target expression'}
                @{Name='--concurrency'; Desc='Number of concurrent executions'}
                @{Name='--command-timeout'; Desc='Command timeout'}
                @{Name='--env'; Desc='Environment variable'}
                @{Name='--job-id'; Desc='Custom batch job ID'}
            )
            'state' = @(
                @{Name='--target'; Desc='Target expression'}
                @{Name='--vars'; Desc='Variables file'}
                @{Name='--dry-run'; Desc='Check what would change'}
            )
            'cluster' = @(
                @{Name='--server'; Desc='Control plane address'}
                @{Name='-s'; Desc='Control plane address'}
                @{Name='--output'; Desc='Output format'}
                @{Name='-o'; Desc='Output format'}
                @{Name='--verbose'; Desc='Enable verbose output'}
                @{Name='-v'; Desc='Enable verbose output'}
            )
            'identity' = @(
                @{Name='--server'; Desc='Control plane API address'}
                @{Name='--output'; Desc='Output format'}
                @{Name='-o'; Desc='Output format'}
            )
            'monitor' = @(
                @{Name='--control-plane'; Desc='Control plane gRPC address'}
                @{Name='--nats-url'; Desc='NATS server URL'}
                @{Name='--theme'; Desc='UI theme'}
                @{Name='--refresh'; Desc='Refresh interval in seconds'}
                @{Name='--no-color'; Desc='Disable colors'}
            )
            'bootstrap' = @(
                @{Name='--output-dir'; Desc='Output directory'}
                @{Name='--dry-run'; Desc='Simulation without execution'}
                @{Name='--force'; Desc='Force operation'}
                @{Name='--skip-verification'; Desc='Skip verification'}
                @{Name='--timeout'; Desc='Operation timeout'}
            )
            'gitops' = @(
                @{Name='--app'; Desc='Application name'}
                @{Name='--namespace'; Desc='Namespace'}
                @{Name='--type'; Desc='Rollback type'}
                @{Name='--strategy'; Desc='Strategy'}
                @{Name='--revision'; Desc='Target revision'}
                @{Name='--reason'; Desc='Reason'}
                @{Name='--dry-run'; Desc='Simulate without executing'}
                @{Name='--output'; Desc='Output format'}
                @{Name='-o'; Desc='Output format'}
                @{Name='--pipeline'; Desc='Pipeline name'}
                @{Name='--from'; Desc='Source environment'}
                @{Name='--to'; Desc='Target environment'}
            )
            'policy' = @(
                @{Name='--category'; Desc='Filter by category'}
                @{Name='--type'; Desc='Filter by type'}
                @{Name='--output'; Desc='Output format'}
                @{Name='-o'; Desc='Output format'}
                @{Name='--policy'; Desc='Policy ID'}
                @{Name='--input-file'; Desc='Input JSON file'}
                @{Name='--input'; Desc='Inline input JSON'}
            )
            'audit' = @(
                @{Name='--server'; Desc='Control plane address'}
                @{Name='-s'; Desc='Control plane address'}
                @{Name='--format'; Desc='Output format'}
                @{Name='-o'; Desc='Output format'}
                @{Name='--policy'; Desc='Filter by policy ID'}
                @{Name='--resource-type'; Desc='Filter by resource type'}
                @{Name='--denied'; Desc='Show only denied evaluations'}
                @{Name='--limit'; Desc='Maximum entries'}
                @{Name='--days'; Desc='Number of days'}
                @{Name='--severity'; Desc='Filter by severity'}
                @{Name='--export-format'; Desc='Export format'}
            )
            'webhook' = @(
                @{Name='--server'; Desc='Control plane address'}
                @{Name='-s'; Desc='Control plane address'}
                @{Name='--format'; Desc='Output format'}
                @{Name='-o'; Desc='Output format'}
                @{Name='--limit'; Desc='Maximum entries'}
            )
            'federation' = @(
                @{Name='--server'; Desc='Control plane API address'}
                @{Name='--output'; Desc='Output format'}
                @{Name='-o'; Desc='Output format'}
                @{Name='--bundle-endpoint'; Desc='URL to fetch trust bundle'}
                @{Name='--type'; Desc='Federation type'}
                @{Name='--refresh-interval'; Desc='Bundle refresh interval'}
            )
            'files' = @(
                @{Name='--config'; Desc='Config file path'}
                @{Name='--nats-url'; Desc='NATS server URL'}
                @{Name='--cluster-id'; Desc='Cluster ID'}
                @{Name='--instance-id'; Desc='Server instance ID'}
            )
        }

        $allFlags = $globalFlags
        if ($cmd -and $cmdFlags.ContainsKey($cmd)) {
            $allFlags += $cmdFlags[$cmd]
        }

        $allFlags | Where-Object { $_.Name -like "$wordToComplete*" } | ForEach-Object {
            [CompletionResult]::new($_.Name, $_.Name, 'ParameterName', $_.Desc)
        }
        return
    }

    # Webhook type completion
    if ($cmd -eq 'webhook' -and ($subcmd -eq 'show' -or $subcmd -eq 'test')) {
        @('argocd', 'flux', 'github', 'gitlab') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [CompletionResult]::new($_, $_, 'ParameterValue', "Webhook type: $_")
        }
        return
    }
}
