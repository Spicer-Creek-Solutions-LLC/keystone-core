def test_shape_is_pure():
    s = shape({"status": 200, "body": "hello"})
    assert.eq(200, s["status"])
    assert.eq(5, s["size"])

# Under `kscore-module test` no HTTP host is wired, so the capability
# fails closed (and is audited) instead of reaching the network.
def test_get_without_host_fails_closed():
    assert.fails(lambda: http.get("https://api.example.com/health"))
