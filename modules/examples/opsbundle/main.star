# keystone/opsbundle — dependency composition.
#
# `dependencies:` in the manifest is a distribution / version-pinning
# relationship: `kscore-module resolve|tree|install` pull the pinned
# companion modules (keystone/httpfetch, keystone/cmdrun) and lock
# their exact versions for reproducible supply chains. (Importing
# another module's code at runtime via Starlark `load()` is a
# post-v1.0 item — see the project ROADMAP.) This entrypoint is
# standalone, pure orchestration.

def plan(targets):
    return [{"step": i, "target": t} for i, t in enumerate(targets)]

def record(run_id, status):
    kv.set("run:" + run_id, status)
    log.info("ops run recorded", run=run_id, status=status)

def main(input):
    steps = plan(input.get("targets", []))
    record(input.get("run_id", "r0"), "planned")
    return {"steps": steps, "count": len(steps)}
