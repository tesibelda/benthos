//go:build x_bm_removed

package all

import (
	// Import extra packages, these are packages only imported with the tag
	// x_bm_removed, moved from package.go to reduce binary size
	// Import all public sub-categories.
	// Import all public sub-categories.
	_ "github.com/benthosdev/benthos/v4/public/components/amqp09"
	_ "github.com/benthosdev/benthos/v4/public/components/avro"
	_ "github.com/benthosdev/benthos/v4/public/components/aws"
	_ "github.com/benthosdev/benthos/v4/public/components/azure"
	_ "github.com/benthosdev/benthos/v4/public/components/beanstalkd"
	_ "github.com/benthosdev/benthos/v4/public/components/cassandra"
	_ "github.com/benthosdev/benthos/v4/public/components/changelog"
	_ "github.com/benthosdev/benthos/v4/public/components/cockroachdb"
	_ "github.com/benthosdev/benthos/v4/public/components/confluent"
	_ "github.com/benthosdev/benthos/v4/public/components/couchbase"
	_ "github.com/benthosdev/benthos/v4/public/components/dgraph"
	_ "github.com/benthosdev/benthos/v4/public/components/discord"
	_ "github.com/benthosdev/benthos/v4/public/components/elasticsearch"
	_ "github.com/benthosdev/benthos/v4/public/components/gcp"
	_ "github.com/benthosdev/benthos/v4/public/components/hdfs"
	_ "github.com/benthosdev/benthos/v4/public/components/javascript"
	_ "github.com/benthosdev/benthos/v4/public/components/maxmind"
	_ "github.com/benthosdev/benthos/v4/public/components/memcached"
	_ "github.com/benthosdev/benthos/v4/public/components/mongodb"
	_ "github.com/benthosdev/benthos/v4/public/components/mqtt"
	_ "github.com/benthosdev/benthos/v4/public/components/msgpack"
	_ "github.com/benthosdev/benthos/v4/public/components/nanomsg"
	_ "github.com/benthosdev/benthos/v4/public/components/otlp"
	_ "github.com/benthosdev/benthos/v4/public/components/pulsar"
	_ "github.com/benthosdev/benthos/v4/public/components/sentry"
	_ "github.com/benthosdev/benthos/v4/public/components/sftp"
	_ "github.com/benthosdev/benthos/v4/public/components/snowflake"
	_ "github.com/benthosdev/benthos/v4/public/components/splunk"
	_ "github.com/benthosdev/benthos/v4/public/components/statsd"
	_ "github.com/benthosdev/benthos/v4/public/components/twitter"
	_ "github.com/benthosdev/benthos/v4/public/components/wasm"
)
