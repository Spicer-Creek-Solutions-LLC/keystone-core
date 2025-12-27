# std/exec - Command execution module
# Provides convenient command execution using exec capability

def run(command, args=[]):
    """Execute a command and return the result.

    Args:
        command: Command to execute
        args: List of arguments (optional)

    Returns:
        dict with keys: exit_code, stdout, stderr

    Requires:
        exec capability
    """
    result = _host_exec(command, args)
    return {
        "exit_code": result.exit_code,
        "stdout": result.stdout,
        "stderr": result.stderr,
    }

def run_with_input(command, stdin, args=[]):
    """Execute a command with stdin and return the result.

    Args:
        command: Command to execute
        stdin: Input to send to command's stdin
        args: List of arguments (optional)

    Returns:
        dict with keys: exit_code, stdout, stderr

    Requires:
        exec capability
    """
    result = _host_exec_with_input(command, stdin, args)
    return {
        "exit_code": result.exit_code,
        "stdout": result.stdout,
        "stderr": result.stderr,
    }

def success(command, args=[]):
    """Execute a command and return True if it succeeded (exit code 0).

    Args:
        command: Command to execute
        args: List of arguments (optional)

    Returns:
        True if exit code is 0, False otherwise

    Requires:
        exec capability
    """
    result = run(command, args)
    return result["exit_code"] == 0

def output(command, args=[]):
    """Execute a command and return its stdout.

    Args:
        command: Command to execute
        args: List of arguments (optional)

    Returns:
        Command stdout as string

    Raises:
        Error if command fails (non-zero exit code)

    Requires:
        exec capability
    """
    result = run(command, args)
    if result["exit_code"] != 0:
        fail("Command failed with exit code %d: %s" % (result["exit_code"], result["stderr"]))
    return result["stdout"]

def which(command):
    """Check if a command is available in PATH.

    Args:
        command: Command name to check

    Returns:
        True if command is available, False otherwise

    Requires:
        exec capability
    """
    return success("which", [command])

# Export public API
exec = struct(
    run=run,
    run_with_input=run_with_input,
    success=success,
    output=output,
    which=which,
)
