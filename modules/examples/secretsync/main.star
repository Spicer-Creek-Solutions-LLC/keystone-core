# keystone/secretsync — scoped secret read → transform → write.
#
# secrets.read is scoped to `app/source/*` and secrets.write to
# `app/dest/*`; the runtime denies (and audits) any path outside its
# scope. The live secrets path is shown in the Go example test with
# an injected in-memory secrets host.

def sync(src, dst):
    val = secrets.read(src)
    out = {}
    for k in val:
        out[k] = val[k]
    out["rotated"] = "true"
    secrets.write(dst, out)
    log.info("secret synced", src=src, dst=dst)
    return {"keys": sorted(val.keys())}

def main(input):
    return sync(input["src"], input["dst"])
