//go:build !arm

package couchbase

import (
	// Bring in the internal plugin definitions.
	_ "github.com/benthosdev/benthos/v4/internal/impl/couchbase"
)
