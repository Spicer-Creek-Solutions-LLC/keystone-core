# std/crypto - Cryptography module
# Provides hashing functions using exec capability

def sha256(data):
    """Compute SHA256 hash of data.

    Args:
        data: Data to hash (string or bytes)

    Returns:
        Hex-encoded hash string

    Requires:
        exec and fs.write capabilities
    """
    return _host_crypto_sha256(data)

def sha256_file(path):
    """Compute SHA256 hash of a file.

    Args:
        path: Path to file

    Returns:
        Hex-encoded hash string

    Requires:
        fs.read, exec, and fs.write capabilities
    """
    data = _host_fs_read(path)
    return sha256(data)

def verify_sha256(data, expected_hash):
    """Verify SHA256 hash of data.

    Args:
        data: Data to verify (string or bytes)
        expected_hash: Expected hex-encoded hash

    Returns:
        True if hash matches, False otherwise

    Requires:
        exec and fs.write capabilities
    """
    actual_hash = sha256(data)
    return actual_hash.lower() == expected_hash.lower()

# Export public API
crypto = struct(
    sha256=sha256,
    sha256_file=sha256_file,
    verify_sha256=verify_sha256,
)
