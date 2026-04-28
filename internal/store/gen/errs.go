package storedb

import "github.com/jackc/pgx/v5"

// ErrNoRows is re-exported so callers don't need to import pgx directly
// to check for sqlc-generated :one queries returning no row.
var ErrNoRows = pgx.ErrNoRows
