def test_set_get_roundtrip():
    cache_set("t", "a", "1")
    assert.eq("1", cache_get("t", "a"))

def test_incr_accumulates():
    assert.eq(1, incr("counter", "x"))
    assert.eq(2, incr("counter", "x"))

def test_missing_key_is_none():
    assert.eq(None, cache_get("t", "does-not-exist"))

def test_main_set_then_get():
    main({"op": "set", "namespace": "m", "key": "k", "value": "v"})
    assert.eq("v", main({"op": "get", "namespace": "m", "key": "k"})["value"])

def test_log_capability_does_not_fail():
    log.info("kvcache test", phase="unit")
