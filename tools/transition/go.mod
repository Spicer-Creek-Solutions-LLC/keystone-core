// The Generation 2 transition tool is its own module so it stays buildable
// after R08 removes the Generation 1 root module. It deliberately depends on
// nothing outside the standard library: a tool whose job is to mutate the
// issue tracker exactly once, safely, should not carry a dependency graph that
// can drift or fail to resolve years from now when the archive is being
// audited.
module go.keystone-core.io/keystone-core/tools/transition

go 1.27
