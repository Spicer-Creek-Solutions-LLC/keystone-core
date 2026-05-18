# No exec host is wired under `kscore-module test`, so every exec.run
# fails closed (and is audited). The allowlist-allow path is shown in
# the Go example test with an injected exec host.

def test_allowed_command_without_host_fails_closed():
    assert.fails(lambda: run("echo", ["hi"]))

def test_denied_command_fails():
    assert.fails(lambda: run("rm", ["-rf", "/"]))

def test_main_requires_cmd():
    assert.fails(lambda: main({}))
