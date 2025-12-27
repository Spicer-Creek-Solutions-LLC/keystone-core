# std/http - HTTP operations module
# Provides convenient HTTP client using http capability

def get(url):
    """Perform an HTTP GET request.

    Args:
        url: URL to request

    Returns:
        dict with keys: status_code, headers, body

    Requires:
        http.get capability
    """
    response = _host_http_get(url)
    return {
        "status_code": response.status_code,
        "headers": response.headers,
        "body": response.body,
    }

def post(url, body):
    """Perform an HTTP POST request.

    Args:
        url: URL to request
        body: Request body (string or bytes)

    Returns:
        dict with keys: status_code, headers, body

    Requires:
        http.post capability
    """
    response = _host_http_post(url, body)
    return {
        "status_code": response.status_code,
        "headers": response.headers,
        "body": response.body,
    }

def get_text(url):
    """Perform an HTTP GET and return response body as text.

    Args:
        url: URL to request

    Returns:
        Response body as string

    Requires:
        http.get capability
    """
    response = get(url)
    return response["body"]

def get_json(url):
    """Perform an HTTP GET and parse JSON response.

    Args:
        url: URL to request

    Returns:
        Parsed JSON object

    Requires:
        http.get capability
    """
    response = get(url)
    return json.decode(response["body"])

def post_json(url, data):
    """Perform an HTTP POST with JSON body.

    Args:
        url: URL to request
        data: Object to serialize as JSON

    Returns:
        dict with keys: status_code, headers, body

    Requires:
        http.post capability
    """
    body = json.encode(data)
    return post(url, body)

def is_success(response):
    """Check if an HTTP response indicates success (2xx status code).

    Args:
        response: Response dict from get() or post()

    Returns:
        True if status code is 2xx, False otherwise
    """
    return 200 <= response["status_code"] < 300

# Export public API
http = struct(
    get=get,
    post=post,
    get_text=get_text,
    get_json=get_json,
    post_json=post_json,
    is_success=is_success,
)
