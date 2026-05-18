# keystone/kvcache — stateful in-process cache + counter.
#
# Demonstrates two side-effecting capabilities that are fully
# exercisable in `kscore-module test`: `kv` (in-process store) and
# `log` (structured, rate-limited log sink).

def _key(ns, k):
    return ns + ":" + k

def cache_set(ns, k, v):
    kv.set(_key(ns, k), v)
    log.debug("cache set", ns=ns, key=k)

def cache_get(ns, k):
    return kv.get(_key(ns, k))

def incr(ns, k):
    cur = kv.get(_key(ns, k))
    n = (int(cur) if cur != None else 0) + 1
    kv.set(_key(ns, k), str(n))
    return n

def main(input):
    ns = input.get("namespace", "default")
    op = input.get("op", "get")
    key = input.get("key", "")
    if op == "set":
        cache_set(ns, key, input.get("value", ""))
        return {"ok": True}
    if op == "incr":
        return {"value": incr(ns, key)}
    return {"value": cache_get(ns, key)}
