// Package config provides bounded Hujson config reads and patch operations.
//
// It accepts Hujson on reads, enforces a finite byte cap at the read boundary,
// and applies RFC 6902 patches through the Hujson AST so comments survive
// human-maintained config mutations.
package config
