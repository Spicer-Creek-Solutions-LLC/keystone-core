# keystone/hello — a minimal, deterministic, zero-capability module.
#
# A module that requests no capabilities can have no side effects: the
# safest possible default. It only transforms its input into its output.

def greet(name):
    if not name:
        name = "world"
    return "hello, %s!" % name

def main(input):
    return {"message": greet(input.get("name", ""))}
