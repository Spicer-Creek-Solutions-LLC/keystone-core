#compdef kscorectl

# zsh completion for kscorectl

_kscorectl() {
    local -a commands
    local -a global_flags

    commands=(
        'exec:Execute commands on remote agents'
        'state:Manage declarative state configurations'
        'monitor:Real-time TUI monitoring dashboard'
        'module:Manage Keystone Core modules'
        'policy:Policy enforcement and compliance'
        'gitops:GitOps integration commands'
        'version:Show version information'
        'help:Show help information'
    )

    global_flags=(
        '--config[Path to configuration file]:file:_files'
        '--log-level[Set log level]:level:(debug info warn error)'
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
                    _arguments \
                        '--target[Target expression]:target:' \
                        '--timeout[Execution timeout]:timeout:' \
                        '--async[Run asynchronously]' \
                        '1:subcommand:(run async status output)' \
                        '*:arguments:'
                    ;;
                state)
                    _arguments \
                        '--vars[Variables file]:file:_files' \
                        '--dry-run[Show what would be done]' \
                        '1:subcommand:(apply check drift list-modules)' \
                        '*:file:_files -g "*.yaml *.yml"'
                    ;;
                monitor)
                    _arguments \
                        '--server[Server URL]:url:' \
                        '--refresh[Refresh interval]:interval:'
                    ;;
                module)
                    _arguments \
                        '1:subcommand:(init build test sign publish install resolve update tree verify clean mirror)' \
                        '*:arguments:'
                    ;;
                policy)
                    _arguments \
                        '1:subcommand:(check enforce audit report list)' \
                        '*:arguments:'
                    ;;
                gitops)
                    _arguments \
                        '1:subcommand:(verify rollback sync diff status)' \
                        '*:arguments:'
                    ;;
            esac
            ;;
    esac
}

_kscorectl "$@"
