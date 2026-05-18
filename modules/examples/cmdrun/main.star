# keystone/cmdrun — allowlisted command execution.
#
# The manifest's `commands` allowlist + `working_dir` + timeout are
# enforced by the runtime; a command not on the list fails closed and
# is audited. The live `exec.run` path is shown in the Go example
# test with an injected exec host.

def run(cmd, args):
    res = exec.run(cmd, args)
    return {"stdout": res["stdout"], "stderr": res["stderr"]}

def main(input):
    return run(input["cmd"], input.get("args", []))
