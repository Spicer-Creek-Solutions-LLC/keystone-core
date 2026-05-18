def test_plan_is_ordered():
    p = plan(["a", "b", "c"])
    assert.eq(3, len(p))
    assert.eq(0, p[0]["step"])
    assert.eq("c", p[2]["target"])

def test_plan_empty():
    assert.eq(0, len(plan([])))

def test_record_roundtrip():
    record("r1", "done")
    assert.eq("done", kv.get("run:r1"))

def test_main_counts_steps():
    out = main({"targets": ["x", "y"], "run_id": "r2"})
    assert.eq(2, out["count"])
