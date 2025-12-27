# std/json - JSON encoding/decoding module
# Uses built-in Starlark json module

def encode(obj):
    """Encode an object as JSON.

    Args:
        obj: Object to encode (dict, list, string, number, bool, None)

    Returns:
        JSON string

    Raises:
        Error if object cannot be encoded
    """
    return json.encode(obj)

def decode(text):
    """Decode a JSON string.

    Args:
        text: JSON string to decode

    Returns:
        Decoded object

    Raises:
        Error if JSON is invalid
    """
    return json.decode(text)

def indent(obj, indent="  "):
    """Encode an object as indented JSON.

    Args:
        obj: Object to encode
        indent: Indentation string (default: two spaces)

    Returns:
        Pretty-printed JSON string
    """
    return json.encode_indent(obj, indent=indent)

# Export public API
json_module = struct(
    encode=encode,
    decode=decode,
    indent=indent,
)
