// capcheck is its own module so it survives the removal of the Generation 1
// root module. It depends on nothing outside the standard library, and it reads
// the archived sources from the pinned Generation 1 commit rather than from the
// working tree — so deleting those files from the tip does not affect it.
//
// It has a different lifetime from tools/transition. That tool is explicitly
// temporary; this one guards an artifact the project keeps.
module go.keystone-core.io/keystone-core/tools/capcheck

go 1.27
