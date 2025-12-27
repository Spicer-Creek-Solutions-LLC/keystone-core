# bash completion for kscorectl

_kscorectl_completions()
{
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    # Top-level commands
    local commands="exec state monitor module policy gitops version help"

    # Common flags
    local global_flags="--config --log-level --help"

    case "${prev}" in
        kscorectl)
            COMPREPLY=( $(compgen -W "${commands}" -- ${cur}) )
            return 0
            ;;
        exec)
            COMPREPLY=( $(compgen -W "run async status output --target --timeout --help" -- ${cur}) )
            return 0
            ;;
        state)
            COMPREPLY=( $(compgen -W "apply check drift list-modules --vars --dry-run --help" -- ${cur}) )
            return 0
            ;;
        monitor)
            COMPREPLY=( $(compgen -W "--server --refresh --help" -- ${cur}) )
            return 0
            ;;
        module)
            COMPREPLY=( $(compgen -W "init build test sign publish install resolve update tree verify clean mirror --help" -- ${cur}) )
            return 0
            ;;
        policy)
            COMPREPLY=( $(compgen -W "check enforce audit report list --help" -- ${cur}) )
            return 0
            ;;
        gitops)
            COMPREPLY=( $(compgen -W "verify rollback sync diff status --help" -- ${cur}) )
            return 0
            ;;
        --log-level)
            COMPREPLY=( $(compgen -W "debug info warn error" -- ${cur}) )
            return 0
            ;;
        --target)
            # Could add agent completion here
            return 0
            ;;
        *)
            ;;
    esac

    # Default to global flags
    if [[ ${cur} == -* ]]; then
        COMPREPLY=( $(compgen -W "${global_flags}" -- ${cur}) )
        return 0
    fi
}

complete -F _kscorectl_completions kscorectl
