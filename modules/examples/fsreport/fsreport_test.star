# `fs` is os-backed in `kscore-module test`, but the capability scope
# (manifest `paths`) is still enforced — so an out-of-scope path fails
# closed and is audited without ever touching the filesystem.

def test_summarize_pure():
    s = summarize("a\n\nb")
    assert.eq(3, s["lines"])
    assert.eq(2, s["nonempty"])
    assert.eq(4, s["bytes"])

def test_write_outside_scope_is_denied():
    assert.fails(lambda: fs.write("/etc/keystone-denied.txt", "x"))

def test_read_outside_scope_is_denied():
    assert.fails(lambda: fs.read("/etc/shadow"))
