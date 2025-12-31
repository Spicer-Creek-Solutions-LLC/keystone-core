# Hello World Keystone Core Module (Starlark)
#
# This example module demonstrates the Keystone Core Starlark module system by:
# - Getting the CPU make and model
# - Computing a SHA256 hash of the CPU information
# - Writing the results to a file in the temp directory
# - Returning the results as JSON

load("//stdlib/exec", "exec")
load("//stdlib/files", "files")
load("//stdlib/crypto", "crypto")
load("//stdlib/json", "json")

def get_cpu_info():
    """Get CPU make and model information."""
    # Try Linux first
    if files.exists("/proc/cpuinfo"):
        cpuinfo = files.read("/proc/cpuinfo")
        for line in cpuinfo.split("\n"):
            if line.startswith("model name"):
                parts = line.split(":")
                if len(parts) > 1:
                    return parts[1].strip()

    # Try macOS
    result = exec.run("sysctl", ["-n", "machdep.cpu.brand_string"])
    if result["exit_code"] == 0:
        return result["stdout"].strip()

    # Try Windows
    result = exec.run("wmic", ["cpu", "get", "name"])
    if result["exit_code"] == 0:
        lines = result["stdout"].split("\n")
        if len(lines) > 1:
            return lines[1].strip()

    return "Unknown CPU"

def main():
    """Main entry point for the hello world module."""
    log.info("Hello World Starlark module starting")

    # Get CPU information
    log.info("Getting CPU information...")
    cpu_info = get_cpu_info()
    log.info("CPU: %s" % cpu_info)

    # Compute SHA256 hash of CPU info
    log.info("Computing SHA256 hash...")
    hash = crypto.sha256(cpu_info)
    log.info("Hash: %s" % hash)

    # Determine temp directory path (assume Unix for Starlark)
    temp_dir = "/tmp"
    file_path = "%s/hello-from-kscore-starlark.txt" % temp_dir

    # Create file contents
    contents = "CPU: %s\nSHA256: %s\n" % (cpu_info, hash)

    # Write to file
    log.info("Writing to file: %s" % file_path)
    files.write(file_path, contents)
    log.info("File written successfully")

    # Return results as JSON
    result = {
        "cpu_info": cpu_info,
        "hash": hash,
        "file_path": file_path,
    }

    result_json = json.encode(result)
    log.info("Result: %s" % result_json)
    log.info("Hello World Starlark module completed successfully")

    return result

# Run the module
main()
