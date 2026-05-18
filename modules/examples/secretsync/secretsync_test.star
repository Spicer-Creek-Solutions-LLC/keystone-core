# No secrets host is wired under `kscore-module test`, so the
# capability fails closed (and is audited). The scoped read→write
# round-trip and the cross-scope denial are shown in the Go example
# test with an injected in-memory secrets host.

def test_read_without_host_fails_closed():
    assert.fails(lambda: secrets.read("app/source/db"))

def test_write_without_host_fails_closed():
    assert.fails(lambda: secrets.write("app/dest/db", {"k": "v"}))

def test_main_requires_src_and_dst():
    assert.fails(lambda: main({"src": "app/source/db"}))
