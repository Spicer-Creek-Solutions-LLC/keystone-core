# Unit tests run by `kscore-module test`. Every top-level test_*
# function is a test; module functions are in scope directly.

def test_default_greeting():
    assert.eq("hello, world!", greet(""))

def test_named_greeting():
    assert.eq("hello, ada!", greet("ada"))

def test_main_uses_input():
    out = main({"name": "ops"})
    assert.eq("hello, ops!", out["message"])

def test_main_defaults_when_empty():
    assert.eq("hello, world!", main({})["message"])
