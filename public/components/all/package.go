// Package all imports all component implementations that ship with the open
// source Benthos repo. This is a convenient way of importing every single
// connector at the cost of a larger dependency tree for your application.
package all

import (
	// Import all public sub-categories.
	_ "github.com/benthosdev/benthos/v4/public/components/amqp1"
	_ "github.com/benthosdev/benthos/v4/public/components/crypto"
	_ "github.com/benthosdev/benthos/v4/public/components/influxdb"
	_ "github.com/benthosdev/benthos/v4/public/components/io"
	_ "github.com/benthosdev/benthos/v4/public/components/jaeger"
	_ "github.com/benthosdev/benthos/v4/public/components/kafka"
	_ "github.com/benthosdev/benthos/v4/public/components/nats"
	_ "github.com/benthosdev/benthos/v4/public/components/nsq"
	_ "github.com/benthosdev/benthos/v4/public/components/opensearch"
	_ "github.com/benthosdev/benthos/v4/public/components/prometheus"
	_ "github.com/benthosdev/benthos/v4/public/components/pure"
	_ "github.com/benthosdev/benthos/v4/public/components/pure/extended"
	_ "github.com/benthosdev/benthos/v4/public/components/pusher"
	_ "github.com/benthosdev/benthos/v4/public/components/redis"
	_ "github.com/benthosdev/benthos/v4/public/components/sql"
)
