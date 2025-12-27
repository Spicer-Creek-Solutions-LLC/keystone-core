# std/strings - String manipulation module
# Pure Starlark string utilities (no capabilities required)

def upper(s):
    """Convert string to uppercase.

    Args:
        s: String to convert

    Returns:
        Uppercase string
    """
    return s.upper()

def lower(s):
    """Convert string to lowercase.

    Args:
        s: String to convert

    Returns:
        Lowercase string
    """
    return s.lower()

def title(s):
    """Convert string to title case.

    Args:
        s: String to convert

    Returns:
        Title case string
    """
    return s.title()

def trim(s):
    """Remove leading and trailing whitespace.

    Args:
        s: String to trim

    Returns:
        Trimmed string
    """
    return s.strip()

def trim_left(s):
    """Remove leading whitespace.

    Args:
        s: String to trim

    Returns:
        Left-trimmed string
    """
    return s.lstrip()

def trim_right(s):
    """Remove trailing whitespace.

    Args:
        s: String to trim

    Returns:
        Right-trimmed string
    """
    return s.rstrip()

def split(s, sep):
    """Split string by separator.

    Args:
        s: String to split
        sep: Separator string

    Returns:
        List of substrings
    """
    return s.split(sep)

def join(items, sep):
    """Join items with separator.

    Args:
        items: List of strings to join
        sep: Separator string

    Returns:
        Joined string
    """
    return sep.join(items)

def replace(s, old, new):
    """Replace all occurrences of old with new.

    Args:
        s: String to modify
        old: Substring to replace
        new: Replacement substring

    Returns:
        Modified string
    """
    return s.replace(old, new)

def contains(s, substr):
    """Check if string contains substring.

    Args:
        s: String to search
        substr: Substring to find

    Returns:
        True if substring is found, False otherwise
    """
    return substr in s

def has_prefix(s, prefix):
    """Check if string starts with prefix.

    Args:
        s: String to check
        prefix: Prefix to find

    Returns:
        True if string starts with prefix, False otherwise
    """
    return s.startswith(prefix)

def has_suffix(s, suffix):
    """Check if string ends with suffix.

    Args:
        s: String to check
        suffix: Suffix to find

    Returns:
        True if string ends with suffix, False otherwise
    """
    return s.endswith(suffix)

def trim_prefix(s, prefix):
    """Remove prefix from string if present.

    Args:
        s: String to modify
        prefix: Prefix to remove

    Returns:
        String without prefix
    """
    if has_prefix(s, prefix):
        return s[len(prefix):]
    return s

def trim_suffix(s, suffix):
    """Remove suffix from string if present.

    Args:
        s: String to modify
        suffix: Suffix to remove

    Returns:
        String without suffix
    """
    if has_suffix(s, suffix):
        return s[:-len(suffix)]
    return s

def repeat(s, count):
    """Repeat string count times.

    Args:
        s: String to repeat
        count: Number of times to repeat

    Returns:
        Repeated string
    """
    return s * count

def reverse(s):
    """Reverse a string.

    Args:
        s: String to reverse

    Returns:
        Reversed string
    """
    chars = list(s)
    chars.reverse()
    return "".join(chars)

# Export public API
strings = struct(
    upper=upper,
    lower=lower,
    title=title,
    trim=trim,
    trim_left=trim_left,
    trim_right=trim_right,
    split=split,
    join=join,
    replace=replace,
    contains=contains,
    has_prefix=has_prefix,
    has_suffix=has_suffix,
    trim_prefix=trim_prefix,
    trim_suffix=trim_suffix,
    repeat=repeat,
    reverse=reverse,
)
