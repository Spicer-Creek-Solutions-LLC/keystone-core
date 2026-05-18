# keystone/fsreport — scoped filesystem read/write.
#
# The capability layer enforces the manifest's fs.read/fs.write
# `paths` (and `denied_paths` / `max_file_size`): a path outside the
# allowed glob fails closed and is audited, even though the module
# code itself contains no such check. summarize() is pure.

def summarize(text):
    lines = text.split("\n")
    nonempty = [l for l in lines if l != ""]
    return {"lines": len(lines), "nonempty": len(nonempty), "bytes": len(text)}

def report(src, dst):
    summary = summarize(fs.read(src))
    fs.write(dst, "lines=%d nonempty=%d bytes=%d\n" % (
        summary["lines"], summary["nonempty"], summary["bytes"]))
    return summary

def main(input):
    return report(input["src"], input["dst"])
