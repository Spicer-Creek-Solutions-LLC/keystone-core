#compdef kscorectl

# zsh completion for kscorectl and kscore-* plugins
# Place in $fpath or source directly

_kscorectl() {
    local -a commands
    local -a global_flags

    commands=(
        'exec:Execute commands on remote agents'
        'state:Manage declarative state configurations'
        'module:Manage Keystone Core modules'
        'policy:Policy enforcement and compliance'
        'gitops:GitOps integration and deployment'
        'blueprint:Manage reusable infrastructure patterns'
        'cluster:Manage Keystone Core cluster'
        'identity:SPIFFE identity and certificate management'
        'files:File distribution server'
        'audit:Policy audit and compliance reporting'
        'webhook:Webhook handler management'
        'federation:Trust federation management'
        'bootstrap:Cluster bootstrap and initialization'
        'monitor:Real-time TUI monitoring dashboard'
        'migrate:Database migration utilities'
        'version:Show version information'
        'help:Show help information'
    )

    global_flags=(
        '--config[Path to configuration file]:file:_files'
        '--log-level[Set log level]:level:(debug info warn error)'
        '--audit-level[Audit logging level]:level:(debug info warn error all)'
        '--audit-output[Audit output backend]:backend:(auto file syslog)'
        '--help[Show help]'
    )

    _arguments -C \
        $global_flags \
        '1:command:->command' \
        '*::arg:->args'

    case "$state" in
        command)
            _describe -t commands 'kscorectl commands' commands
            ;;
        args)
            case "${words[1]}" in
                exec)
                    _kscorectl_exec
                    ;;
                state)
                    _kscorectl_state
                    ;;
                module)
                    _kscorectl_module
                    ;;
                policy)
                    _kscorectl_policy
                    ;;
                gitops)
                    _kscorectl_gitops
                    ;;
                blueprint)
                    _kscorectl_blueprint
                    ;;
                cluster)
                    _kscorectl_cluster
                    ;;
                identity)
                    _kscorectl_identity
                    ;;
                files)
                    _kscorectl_files
                    ;;
                audit)
                    _kscorectl_audit
                    ;;
                webhook)
                    _kscorectl_webhook
                    ;;
                federation)
                    _kscorectl_federation
                    ;;
                bootstrap)
                    _kscorectl_bootstrap
                    ;;
                monitor)
                    _kscorectl_monitor
                    ;;
            esac
            ;;
    esac
}

_kscorectl_exec() {
    local -a subcommands
    subcommands=(
        'run:Execute command across matching agents'
        'status:Get batch job status'
        'list:List batch jobs'
        'version:Print version information'
    )

    _arguments -C \
        '--server[Keystone Core server address]:url:' \
        '--timeout[Request timeout]:timeout:' \
        '--tls[Enable TLS]' \
        '--tls-ca-cert[Path to CA certificate]:file:_files' \
        '--tls-cert[Path to client certificate]:file:_files' \
        '--tls-key[Path to client private key]:file:_files' \
        '--tls-skip-verify[Skip TLS verification]' \
        '--tls-server-name[Server name for TLS verification]:name:' \
        '1:subcommand:->subcommand' \
        '*::arg:->args'

    case "$state" in
        subcommand)
            _describe -t subcommands 'exec subcommands' subcommands
            ;;
        args)
            case "${words[1]}" in
                run)
                    _arguments \
                        '--concurrency[Number of concurrent executions]:number:' \
                        '--continue-on-failure[Continue on errors]' \
                        '--target[Target expression]:target:' \
                        '--working-dir[Working directory]:dir:_directories' \
                        '--user[User to run as]:user:' \
                        '--command-timeout[Command timeout]:timeout:' \
                        '*--env[Environment variable]:var:' \
                        '--job-id[Custom batch job ID]:id:' \
                        '--show-progress[Show progress]' \
                        '--show-results[Show per-agent results]' \
                        '*:command:'
                    ;;
                status)
                    _arguments '*:job-id:'
                    ;;
                list)
                    _arguments \
                        '--status[Filter by status]:status:(pending running completed failed)' \
                        '--page-size[Number of jobs to return]:number:'
                    ;;
            esac
            ;;
    esac
}

_kscorectl_state() {
    local -a subcommands
    subcommands=(
        'apply:Apply state declarations'
        'check:Check state without applying (dry-run)'
        'drift:Detect drift from desired state'
        'version:Print version information'
    )

    _arguments -C \
        '1:subcommand:->subcommand' \
        '*::arg:->args'

    case "$state" in
        subcommand)
            _describe -t subcommands 'state subcommands' subcommands
            ;;
        args)
            case "${words[1]}" in
                apply)
                    _arguments \
                        '--target[Target expression]:target:' \
                        '--vars[Variables file]:file:_files -g "*.yaml *.yml"' \
                        '--dry-run[Check what would change]' \
                        '*:statefile:_files -g "*.yaml *.yml"'
                    ;;
                check|drift)
                    _arguments \
                        '--target[Target expression]:target:' \
                        '--vars[Variables file]:file:_files -g "*.yaml *.yml"' \
                        '*:statefile:_files -g "*.yaml *.yml"'
                    ;;
            esac
            ;;
    esac
}

_kscorectl_module() {
    local -a subcommands
    subcommands=(
        'init:Create new module from template'
        'validate:Check module.yaml syntax'
        'build:Package module as ZIP'
        'resolve:Resolve dependencies'
        'tree:Display dependency tree'
        'verify:Verify cryptographic signatures'
        'sign:Sign module with private key'
        'test:Run module tests'
        'publish:Publish to registry'
        'install:Install from registry'
        'version:Print version information'
    )

    _arguments -C \
        '1:subcommand:->subcommand' \
        '*::arg:->args'

    case "$state" in
        subcommand)
            _describe -t subcommands 'module subcommands' subcommands
            ;;
    esac
}

_kscorectl_policy() {
    local -a subcommands
    subcommands=(
        'list:List policies from file'
        'validate:Validate policy syntax'
        'check:Evaluate policy against input'
        'show:Show policy details'
        'audit:Show audit log (deprecated)'
        'report:Generate compliance report (deprecated)'
        'version:Print version information'
    )

    _arguments -C \
        '1:subcommand:->subcommand' \
        '*::arg:->args'

    case "$state" in
        subcommand)
            _describe -t subcommands 'policy subcommands' subcommands
            ;;
        args)
            case "${words[1]}" in
                list)
                    _arguments \
                        '--category[Filter by category]:category:' \
                        '--type[Filter by type]:type:(opa cel)' \
                        '(-o --output)'{-o,--output}'[Output format]:format:(table text json yaml)' \
                        '*:policyfile:_files -g "*.yaml *.yml *.rego"'
                    ;;
                check)
                    _arguments \
                        '--policy[Policy ID to evaluate]:policy:' \
                        '--input-file[Input JSON file]:file:_files -g "*.json"' \
                        '--input[Inline input JSON]:json:' \
                        '--action[Action being performed]:action:' \
                        '--user[User performing action]:user:' \
                        '--context[Additional context as JSON]:json:' \
                        '(-o --output)'{-o,--output}'[Output format]:format:(text json yaml)' \
                        '*:policyfile:_files -g "*.yaml *.yml *.rego"'
                    ;;
                show)
                    _arguments \
                        '(-o --output)'{-o,--output}'[Output format]:format:(text json yaml)' \
                        '1:policyfile:_files -g "*.yaml *.yml *.rego"' \
                        '2:policyid:'
                    ;;
                audit)
                    _arguments \
                        '--policy[Filter by policy ID]:policy:' \
                        '--resource-type[Filter by resource type]:type:' \
                        '--denied[Show only denied evaluations]' \
                        '--limit[Maximum entries]:number:' \
                        '(-o --output)'{-o,--output}'[Output format]:format:(table text json yaml)'
                    ;;
                report)
                    _arguments \
                        '--days[Number of days]:days:' \
                        '(-o --output)'{-o,--output}'[Output format]:format:(text json yaml)'
                    ;;
            esac
            ;;
    esac
}

_kscorectl_gitops() {
    local -a subcommands
    subcommands=(
        'verify:Run verification workflow'
        'rollback:Trigger rollback operation'
        'promote:Promote between environments'
        'status:Show GitOps operation status'
        'webhook:Webhook commands (deprecated)'
        'version:Print version information'
    )

    _arguments -C \
        '1:subcommand:->subcommand' \
        '*::arg:->args'

    case "$state" in
        subcommand)
            _describe -t subcommands 'gitops subcommands' subcommands
            ;;
        args)
            case "${words[1]}" in
                verify)
                    _arguments \
                        '--parallel[Run steps in parallel]' \
                        '--timeout[Workflow timeout]:timeout:' \
                        '(-o --output)'{-o,--output}'[Output format]:format:(text json yaml)' \
                        '*:workflow-file:_files -g "*.yaml *.yml"'
                    ;;
                rollback)
                    _arguments \
                        '--app[Application name]:app:' \
                        '--namespace[Namespace]:namespace:' \
                        '--type[Rollback type]:type:(argocd flux git)' \
                        '--strategy[Strategy]:strategy:(previous specific last_known_good)' \
                        '--revision[Target revision]:revision:' \
                        '--reason[Reason for rollback]:reason:' \
                        '--user[User performing rollback]:user:' \
                        '--dry-run[Simulate without executing]' \
                        '(-o --output)'{-o,--output}'[Output format]:format:(text json yaml)'
                    ;;
                promote)
                    _arguments \
                        '--pipeline[Pipeline name]:pipeline:' \
                        '--from[Source environment]:env:' \
                        '--to[Target environment]:env:' \
                        '--revision[Specific revision to promote]:revision:' \
                        '--reason[Reason for promotion]:reason:' \
                        '--user[User performing promotion]:user:' \
                        '--skip-verify[Skip verification step]' \
                        '--force[Force promotion despite failures]' \
                        '--dry-run[Simulate without executing]' \
                        '(-o --output)'{-o,--output}'[Output format]:format:(text json yaml)'
                    ;;
                status)
                    _arguments \
                        '--type[Status type]:type:(rollbacks promotions verifications all)' \
                        '--limit[Maximum entries]:number:' \
                        '(-o --output)'{-o,--output}'[Output format]:format:(text json yaml)'
                    ;;
            esac
            ;;
    esac
}

_kscorectl_blueprint() {
    local -a subcommands
    subcommands=(
        'init:Create new blueprint'
        'validate:Check blueprint syntax'
        'lint:Run best practices checks'
        'test:Run blueprint tests'
        'search:Search for blueprints'
        'info:Show blueprint details'
        'versions:List available versions'
        'install:Install blueprint'
        'update:Update blueprints'
        'remove:Remove blueprint'
        'publish:Publish blueprint (deprecated)'
        'sign:Sign blueprint (deprecated)'
        'verify:Verify signature (deprecated)'
        'docs:Generate documentation (deprecated)'
        'rollback:Rollback version (deprecated)'
        'snapshot:Manage snapshots (deprecated)'
        'version:Print version information'
    )

    _arguments -C \
        '1:subcommand:->subcommand' \
        '*::arg:->args'

    case "$state" in
        subcommand)
            _describe -t subcommands 'blueprint subcommands' subcommands
            ;;
    esac
}

_kscorectl_cluster() {
    local -a subcommands
    subcommands=(
        'status:View cluster status'
        'members:Manage cluster members'
        'leader:Monitor leader election'
        'add:Add cluster member'
        'remove:Remove cluster member'
        'transfer-leader:Transfer leader role'
        'rebalance:Rebalance cluster'
        'backup:Backup cluster (deprecated)'
        'restore:Restore from backup (deprecated)'
        'version:Print version information'
    )

    _arguments -C \
        '(-s --server)'{-s,--server}'[Control plane address]:url:' \
        '(-o --output)'{-o,--output}'[Output format]:format:(table text json yaml)' \
        '(-v --verbose)'{-v,--verbose}'[Enable verbose output]' \
        '1:subcommand:->subcommand' \
        '*::arg:->args'

    case "$state" in
        subcommand)
            _describe -t subcommands 'cluster subcommands' subcommands
            ;;
    esac
}

_kscorectl_identity() {
    local -a subcommands
    subcommands=(
        'status:Show identity provider status'
        'token:Manage join tokens'
        'ca:Certificate authority management'
        'federation:Federation management'
        'bundle:Trust bundle management'
        'events:View identity events'
        'version:Print version information'
    )

    _arguments -C \
        '--server[Control plane API address]:url:' \
        '(-o --output)'{-o,--output}'[Output format]:format:(table text json yaml)' \
        '1:subcommand:->subcommand' \
        '*::arg:->args'

    case "$state" in
        subcommand)
            _describe -t subcommands 'identity subcommands' subcommands
            ;;
        args)
            case "${words[1]}" in
                token)
                    local -a token_cmds
                    token_cmds=(
                        'create:Create join token'
                        'list:List join tokens'
                        'show:Show token details'
                        'revoke:Revoke token'
                    )
                    _arguments -C \
                        '1:subcommand:->subcommand' \
                        '*::arg:->args'
                    case "$state" in
                        subcommand)
                            _describe -t subcommands 'token subcommands' token_cmds
                            ;;
                        args)
                            case "${words[1]}" in
                                create)
                                    _arguments \
                                        '--agent-id[Agent ID]:id:' \
                                        '--ttl[Token time-to-live]:ttl:' \
                                        '*--label[Labels]:label:'
                                    ;;
                            esac
                            ;;
                    esac
                    ;;
                ca)
                    local -a ca_cmds
                    ca_cmds=(
                        'info:Show CA information'
                        'backup:Backup CA certificates'
                        'restore:Restore CA from backup'
                        'rotate:Trigger CA rotation'
                    )
                    _arguments -C \
                        '1:subcommand:->subcommand' \
                        '*::arg:->args'
                    case "$state" in
                        subcommand)
                            _describe -t subcommands 'ca subcommands' ca_cmds
                            ;;
                        args)
                            case "${words[1]}" in
                                backup)
                                    _arguments \
                                        '(-o --output)'{-o,--output}'[Output file]:file:_files' \
                                        '--encrypt[Encrypt backup]'
                                    ;;
                                restore)
                                    _arguments \
                                        '--backup[Backup file to restore]:file:_files'
                                    ;;
                            esac
                            ;;
                    esac
                    ;;
                bundle)
                    local -a bundle_cmds
                    bundle_cmds=(
                        'show:Show local trust bundle'
                        'export:Export trust bundle'
                    )
                    _arguments -C \
                        '1:subcommand:->subcommand' \
                        '*::arg:->args'
                    case "$state" in
                        subcommand)
                            _describe -t subcommands 'bundle subcommands' bundle_cmds
                            ;;
                        args)
                            case "${words[1]}" in
                                export)
                                    _arguments \
                                        '--format[Export format]:format:(pem jwks spiffe)'
                                    ;;
                            esac
                            ;;
                    esac
                    ;;
                events)
                    _arguments \
                        '(-f --follow)'{-f,--follow}'[Follow events in real-time]'
                    ;;
            esac
            ;;
    esac
}

_kscorectl_files() {
    local -a subcommands
    subcommands=(
        'serve:Start the file server'
        'files:File management'
        'cache:Cache management'
        'namespace:Namespace management'
        'backend:Backend configuration (deprecated)'
        'mirrors:Mirror configuration (deprecated)'
        'version:Print version information'
    )

    _arguments -C \
        '--config[Config file path]:file:_files' \
        '--nats-url[NATS server URL]:url:' \
        '--cluster-id[Cluster ID]:id:' \
        '--instance-id[Server instance ID]:id:' \
        '1:subcommand:->subcommand' \
        '*::arg:->args'

    case "$state" in
        subcommand)
            _describe -t subcommands 'files subcommands' subcommands
            ;;
    esac
}

_kscorectl_audit() {
    local -a subcommands
    subcommands=(
        'log:View policy evaluation audit log'
        'report:Generate compliance report'
        'export:Export audit data'
        'stats:View audit statistics and trends'
        'version:Print version information'
    )

    _arguments -C \
        '(-s --server)'{-s,--server}'[Control plane address]:url:' \
        '(-o --format)'{-o,--format}'[Output format]:format:(table text json yaml)' \
        '1:subcommand:->subcommand' \
        '*::arg:->args'

    case "$state" in
        subcommand)
            _describe -t subcommands 'audit subcommands' subcommands
            ;;
        args)
            case "${words[1]}" in
                log)
                    _arguments \
                        '--policy[Filter by policy ID]:policy:' \
                        '--resource-type[Filter by resource type]:type:' \
                        '--denied[Show only denied evaluations]' \
                        '--limit[Maximum entries]:number:' \
                        '--since[Show entries since date]:date:' \
                        '--until[Show entries until date]:date:'
                    ;;
                report)
                    _arguments \
                        '--days[Number of days in report]:days:' \
                        '--category[Filter by policy category]:category:' \
                        '--severity[Filter by severity]:severity:(low medium high critical)'
                    ;;
                export)
                    _arguments \
                        '--days[Number of days to export]:days:' \
                        '--output[Output file]:file:_files' \
                        '--export-format[Format]:format:(json csv yaml)'
                    ;;
                stats)
                    _arguments \
                        '--days[Number of days to analyze]:days:'
                    ;;
            esac
            ;;
    esac
}

_kscorectl_webhook() {
    local -a subcommands
    subcommands=(
        'list:List registered webhook handlers'
        'show:Show webhook handler details'
        'test:Send test webhook'
        'history:View webhook delivery history'
        'secrets:Manage webhook secrets'
        'version:Print version information'
    )

    _arguments -C \
        '(-s --server)'{-s,--server}'[Control plane address]:url:' \
        '(-o --format)'{-o,--format}'[Output format]:format:(table text json yaml)' \
        '1:subcommand:->subcommand' \
        '*::arg:->args'

    case "$state" in
        subcommand)
            _describe -t subcommands 'webhook subcommands' subcommands
            ;;
        args)
            case "${words[1]}" in
                show|test)
                    _arguments \
                        '1:type:(argocd flux github gitlab)'
                    ;;
                history)
                    _arguments \
                        '--limit[Maximum entries to show]:number:'
                    ;;
                secrets)
                    local -a secrets_cmds
                    secrets_cmds=(
                        'list:List webhook secrets'
                        'rotate:Rotate webhook secret'
                    )
                    _arguments -C \
                        '1:subcommand:->subcommand' \
                        '*::arg:->args'
                    case "$state" in
                        subcommand)
                            _describe -t subcommands 'secrets subcommands' secrets_cmds
                            ;;
                    esac
                    ;;
            esac
            ;;
    esac
}

_kscorectl_federation() {
    local -a subcommands
    subcommands=(
        'list:List federated trust domains'
        'add:Add federated domain'
        'show:Show federation details'
        'suspend:Suspend federation'
        'activate:Activate federation'
        'remove:Remove federation'
        'refresh:Refresh trust bundle'
        'bundle:Manage trust bundles'
        'version:Print version information'
    )

    _arguments -C \
        '--server[Control plane API address]:url:' \
        '(-o --output)'{-o,--output}'[Output format]:format:(table text json yaml)' \
        '1:subcommand:->subcommand' \
        '*::arg:->args'

    case "$state" in
        subcommand)
            _describe -t subcommands 'federation subcommands' subcommands
            ;;
        args)
            case "${words[1]}" in
                add)
                    _arguments \
                        '--bundle-endpoint[URL to fetch trust bundle]:url:' \
                        '--type[Federation type]:type:(bidirectional inbound outbound)' \
                        '--refresh-interval[Bundle refresh interval]:interval:' \
                        '1:trust-domain:'
                    ;;
                show|suspend|activate|remove|refresh)
                    _arguments '1:trust-domain:'
                    ;;
            esac
            ;;
    esac
}

_kscorectl_bootstrap() {
    local -a subcommands
    subcommands=(
        'seed:Bootstrap from seed configuration'
        'restore:Restore from backup'
        'import:Import existing installation'
        'validate:Validate configuration'
        'status:Show bootstrap status'
        'cleanup:Clean up bootstrap resources'
        'version:Print version information'
    )

    _arguments -C \
        '1:subcommand:->subcommand' \
        '*::arg:->args'

    case "$state" in
        subcommand)
            _describe -t subcommands 'bootstrap subcommands' subcommands
            ;;
        args)
            case "${words[1]}" in
                seed|restore|import)
                    _arguments \
                        '--output-dir[Output directory]:dir:_directories' \
                        '--dry-run[Simulation without execution]' \
                        '--force[Force operation]' \
                        '--skip-verification[Skip verification]' \
                        '--timeout[Operation timeout]:timeout:' \
                        '*:config-file:_files'
                    ;;
                validate)
                    _arguments '*:config-file:_files'
                    ;;
            esac
            ;;
    esac
}

_kscorectl_monitor() {
    _arguments \
        '--config[Config file path]:file:_files' \
        '--control-plane[Control plane gRPC address]:url:' \
        '--nats-url[NATS server URL]:url:' \
        '--theme[UI theme]:theme:(dark light solarized-dark solarized-light monokai)' \
        '--refresh[Refresh interval in seconds]:seconds:' \
        '--no-color[Disable colors]'
}

_kscorectl "$@"
