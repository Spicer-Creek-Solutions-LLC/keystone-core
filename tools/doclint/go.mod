// doclint holds the standing document sweeps.
//
// Its own module, like tools/capcheck: it depends on nothing outside the
// standard library and has a different lifetime from the product.
//
// It carries SWEEPS ONLY. An acceptance case proves a document was correct when
// it was accepted and then passes forever; a sweep asserts a conclusion has not
// been contradicted SINCE, which is a claim about the future. Every cross-task
// regression in Stage P was caught by a sweep. No per-ADR structural case ever
// caught a later one, and one that failed on a legitimate amendment would be
// disabled rather than fixed.
module go.keystone-core.io/keystone-core/tools/doclint

go 1.27
