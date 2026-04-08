package storage

import "github.com/sriramsme/OnlyAgents/pkg/dbtypes"

// Type aliases — the real implementations live in pkg/dbtypes.
// All existing code using storage.DBTime etc. continues to work unchanged.
type (
	DBTime           = dbtypes.DBTime
	NullDBTime       = dbtypes.NullDBTime
	JSONSlice[T any] = dbtypes.JSONSlice[T]
	JSONMap          = dbtypes.JSONMap
)
