# std/files - File operations module
# Provides convenient file operations using fs capability

def read(path):
    """Read a file and return its contents as a string.

    Args:
        path: Path to the file to read

    Returns:
        File contents as string

    Requires:
        fs.read capability
    """
    return _host_fs_read_string(path)

def read_bytes(path):
    """Read a file and return its contents as bytes.

    Args:
        path: Path to the file to read

    Returns:
        File contents as bytes

    Requires:
        fs.read capability
    """
    return _host_fs_read(path)

def write(path, content):
    """Write a string to a file.

    Args:
        path: Path to the file to write
        content: String content to write

    Requires:
        fs.write capability
    """
    _host_fs_write_string(path, content)

def write_bytes(path, data):
    """Write bytes to a file.

    Args:
        path: Path to the file to write
        data: Bytes to write

    Requires:
        fs.write capability
    """
    _host_fs_write(path, data)

def exists(path):
    """Check if a file or directory exists.

    Args:
        path: Path to check

    Returns:
        True if exists, False otherwise

    Requires:
        fs.read capability
    """
    try:
        _host_fs_read(path)
        return True
    except:
        return False

def read_lines(path):
    """Read a file and return its lines as a list.

    Args:
        path: Path to the file to read

    Returns:
        List of lines (without newlines)

    Requires:
        fs.read capability
    """
    content = read(path)
    return content.split("\n")

def write_lines(path, lines):
    """Write a list of lines to a file.

    Args:
        path: Path to the file to write
        lines: List of strings to write (newlines added automatically)

    Requires:
        fs.write capability
    """
    content = "\n".join(lines)
    write(path, content)

def append(path, content):
    """Append content to a file.

    Args:
        path: Path to the file
        content: String content to append

    Requires:
        fs.read and fs.write capabilities
    """
    existing = ""
    if exists(path):
        existing = read(path)
    write(path, existing + content)

# Export public API
files = struct(
    read=read,
    read_bytes=read_bytes,
    write=write,
    write_bytes=write_bytes,
    exists=exists,
    read_lines=read_lines,
    write_lines=write_lines,
    append=append,
)
