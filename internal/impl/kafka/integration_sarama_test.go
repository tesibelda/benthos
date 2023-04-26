package kafka_test

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benthosdev/benthos/v4/internal/component/output"
	"github.com/benthosdev/benthos/v4/internal/impl/kafka"
	"github.com/benthosdev/benthos/v4/internal/integration"
	"github.com/benthosdev/benthos/v4/internal/manager/mock"
	"github.com/benthosdev/benthos/v4/internal/message"
	"github.com/benthosdev/benthos/v4/public/service"
)

func TestIntegrationSaramaCheckpointOneLockUp(t *testing.T) {
	integration.CheckSkipExact(t)
	t.Parallel()

	pool, err := dockertest.NewPool("")
	require.NoError(t, err)

	pool.MaxWait = time.Minute

	kafkaPort, err := integration.GetFreePort()
	require.NoError(t, err)

	kafkaPortStr := strconv.Itoa(kafkaPort)

	options := &dockertest.RunOptions{
		Repository:   "docker.vectorized.io/vectorized/redpanda",
		Tag:          "latest",
		Hostname:     "redpanda",
		ExposedPorts: []string{"9092"},
		PortBindings: map[docker.Port][]docker.PortBinding{
			"9092/tcp": {{HostIP: "", HostPort: kafkaPortStr}},
		},
		Cmd: []string{
			"redpanda", "start", "--smp 1", "--overprovisioned", "",
			"--kafka-addr 0.0.0.0:9092",
			fmt.Sprintf("--advertise-kafka-addr localhost:%v", kafkaPort),
		},
	}
	resource, err := pool.RunWithOptions(options)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, pool.Purge(resource))
	})

	_ = resource.Expire(900)
	require.NoError(t, pool.Retry(func() error {
		return createKafkaTopic(context.Background(), "localhost:"+kafkaPortStr, "wcotesttopic", 20)
	}))

	dl, exists := t.Deadline()
	if exists {
		dl = dl.Add(-time.Second)
	} else {
		dl = time.Now().Add(time.Minute)
	}
	testCtx, done := context.WithTimeout(context.Background(), time.Until(dl))
	defer done()

	writeCtx, writeDone := context.WithCancel(testCtx)
	defer writeDone()

	// Create data generator stream
	inBuilder := service.NewStreamBuilder()
	require.NoError(t, inBuilder.AddOutputYAML(fmt.Sprintf(`
kafka:
  addresses: [ "localhost:%v" ]
  topic: topic-wcotesttopic
  max_in_flight: 1
`, kafkaPortStr)))

	inFunc, err := inBuilder.AddProducerFunc()
	require.NoError(t, err)

	inStrm, err := inBuilder.Build()
	require.NoError(t, err)
	go func() {
		assert.NoError(t, inStrm.Run(testCtx))
	}()

	// Create two parallel data consumer streams
	var messageCountMut sync.Mutex
	var inMessages, outMessagesOne, outMessagesTwo int

	outBuilderConf := fmt.Sprintf(`
kafka:
  addresses: [ "localhost:%v" ]
  topics: [ topic-wcotesttopic ]
  consumer_group: wcotestgroup
  checkpoint_limit: 1
  start_from_oldest: true
`, kafkaPortStr)

	outBuilder := service.NewStreamBuilder()
	require.NoError(t, outBuilder.AddInputYAML(outBuilderConf))
	require.NoError(t, outBuilder.AddProcessorYAML(`mapping: 'root = content().uppercase()'`))
	require.NoError(t, outBuilder.AddConsumerFunc(func(ctx context.Context, m *service.Message) error {
		messageCountMut.Lock()
		outMessagesOne++
		messageCountMut.Unlock()
		return nil
	}))
	outStrmOne, err := outBuilder.Build()
	require.NoError(t, err)
	go func() {
		assert.NoError(t, outStrmOne.Run(testCtx))
	}()

	outBuilder = service.NewStreamBuilder()
	require.NoError(t, outBuilder.AddInputYAML(outBuilderConf))
	require.NoError(t, outBuilder.AddConsumerFunc(func(ctx context.Context, m *service.Message) error {
		messageCountMut.Lock()
		outMessagesTwo++
		messageCountMut.Unlock()
		return nil
	}))
	outStrmTwo, err := outBuilder.Build()
	require.NoError(t, err)
	go func() {
		assert.NoError(t, outStrmTwo.Run(testCtx))
	}()

	n := 1000
	go func() {
		for {
			for i := 0; i < n; i++ {
				err := inFunc(writeCtx, service.NewMessage(fmt.Appendf(nil, "hello world %v", i)))
				if writeCtx.Err() != nil {
					return
				}
				assert.NoError(t, err)
				messageCountMut.Lock()
				inMessages++
				messageCountMut.Unlock()
				time.Sleep(time.Millisecond * 10)
			}
		}
	}()

	assert.Eventually(t, func() bool {
		messageCountMut.Lock()
		countOne, countTwo := outMessagesOne, outMessagesTwo
		messageCountMut.Unlock()

		t.Logf("count one: %v, count two: %v", countOne, countTwo)
		return countOne > 0 && countTwo > 0
	}, time.Until(dl), time.Millisecond*500)

	var prevOne, prevTwo int
	assert.Never(t, func() bool {
		messageCountMut.Lock()
		countOne, countTwo := outMessagesOne, outMessagesTwo
		messageCountMut.Unlock()

		hasIncreased := countOne > prevOne && countTwo > prevTwo
		prevOne, prevTwo = countOne, countTwo

		t.Logf("count one: %v, count two: %v", countOne, countTwo)
		return !hasIncreased
	}, time.Until(dl)-time.Second, time.Millisecond*500)

	writeDone()
	require.NoError(t, inStrm.Stop(testCtx))

	require.NoError(t, outStrmOne.Stop(testCtx))
	require.NoError(t, outStrmTwo.Stop(testCtx))
	done()
}

func TestIntegrationSaramaRedpanda(t *testing.T) {
	integration.CheckSkip(t)
	t.Parallel()

	pool, err := dockertest.NewPool("")
	require.NoError(t, err)

	pool.MaxWait = time.Minute

	kafkaPort, err := integration.GetFreePort()
	require.NoError(t, err)

	kafkaPortStr := strconv.Itoa(kafkaPort)

	options := &dockertest.RunOptions{
		Repository:   "docker.vectorized.io/vectorized/redpanda",
		Tag:          "latest",
		Hostname:     "redpanda",
		ExposedPorts: []string{"9092"},
		PortBindings: map[docker.Port][]docker.PortBinding{
			"9092/tcp": {{HostIP: "", HostPort: kafkaPortStr}},
		},
		Cmd: []string{
			"redpanda", "start", "--smp 1", "--overprovisioned", "",
			"--kafka-addr 0.0.0.0:9092",
			fmt.Sprintf("--advertise-kafka-addr localhost:%v", kafkaPort),
		},
	}
	resource, err := pool.RunWithOptions(options)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, pool.Purge(resource))
	})

	_ = resource.Expire(900)

	require.NoError(t, pool.Retry(func() error {
		return createKafkaTopic(context.Background(), "localhost:"+kafkaPortStr, "pls_ignore_just_testing_connection", 1)
	}))

	template := `
output:
  kafka:
    addresses: [ localhost:$PORT ]
    topic: topic-$ID
    max_in_flight: $MAX_IN_FLIGHT
    retry_as_batch: $VAR3
    metadata:
      exclude_prefixes: [ $OUTPUT_META_EXCLUDE_PREFIX ]
    batching:
      count: $OUTPUT_BATCH_COUNT

input:
  kafka:
    addresses: [ localhost:$PORT ]
    topics: [ topic-$ID$VAR1 ]
    consumer_group: "$VAR4"
    checkpoint_limit: $VAR2
    start_from_oldest: true
    batching:
      count: $INPUT_BATCH_COUNT
`

	suite := integration.StreamTests(
		integration.StreamTestOpenClose(),
		integration.StreamTestMetadata(),
		integration.StreamTestMetadataFilter(),
		integration.StreamTestSendBatch(10),
		integration.StreamTestStreamSequential(1000),
		integration.StreamTestStreamParallel(1000),
		integration.StreamTestStreamParallelLossy(1000),
		integration.StreamTestSendBatchCount(10),
	)
	// In some modes include testing input level batching
	var suiteExt integration.StreamTestList
	suiteExt = append(suiteExt, suite...)
	suiteExt = append(suiteExt, integration.StreamTestReceiveBatchCount(10))

	// Only for checkpointed tests
	var suiteSingleCheckpointedStream integration.StreamTestList
	suiteSingleCheckpointedStream = append(suiteSingleCheckpointedStream, suite...)
	suiteSingleCheckpointedStream = append(suiteSingleCheckpointedStream, integration.StreamTestCheckpointCapture())

	t.Run("balanced", func(t *testing.T) {
		t.Parallel()
		suite.Run(
			t, template,
			integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
				vars.Var4 = "group" + testID
				require.NoError(t, createKafkaTopic(ctx, "localhost:"+kafkaPortStr, testID, 4))
			}),
			integration.StreamTestOptPort(kafkaPortStr),
			integration.StreamTestOptVarTwo("1"),
			integration.StreamTestOptVarThree("false"),
		)

		t.Run("only one partition", func(t *testing.T) {
			t.Parallel()
			suiteExt.Run(
				t, template,
				integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
					vars.Var4 = "group" + testID
					require.NoError(t, createKafkaTopic(ctx, "localhost:"+kafkaPortStr, testID, 1))
				}),
				integration.StreamTestOptPort(kafkaPortStr),
				integration.StreamTestOptVarTwo("1"),
				integration.StreamTestOptVarThree("false"),
			)
		})

		t.Run("checkpointed", func(t *testing.T) {
			t.Parallel()
			suite.Run(
				t, template,
				integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
					vars.Var4 = "group" + testID
					require.NoError(t, createKafkaTopic(ctx, "localhost:"+kafkaPortStr, testID, 4))
				}),
				integration.StreamTestOptPort(kafkaPortStr),
				integration.StreamTestOptVarTwo("1000"),
				integration.StreamTestOptVarThree("false"),
			)
		})

		t.Run("retry as batch", func(t *testing.T) {
			t.Parallel()
			suite.Run(
				t, template,
				integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
					vars.Var4 = "group" + testID
					require.NoError(t, createKafkaTopic(ctx, "localhost:"+kafkaPortStr, testID, 4))
				}),
				integration.StreamTestOptPort(kafkaPortStr),
				integration.StreamTestOptVarTwo("1"),
				integration.StreamTestOptVarThree("true"),
			)
		})
	})

	t.Run("explicit partitions", func(t *testing.T) {
		t.Parallel()
		suite.Run(
			t, template,
			integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
				vars.Var4 = "group" + testID
				topicName := "topic-" + testID
				vars.Var1 = fmt.Sprintf(":0,%v:1,%v:2,%v:3", topicName, topicName, topicName)
				require.NoError(t, createKafkaTopic(ctx, "localhost:"+kafkaPortStr, testID, 4))
			}),
			integration.StreamTestOptPort(kafkaPortStr),
			integration.StreamTestOptSleepAfterInput(time.Second*3),
			integration.StreamTestOptVarTwo("1"),
			integration.StreamTestOptVarThree("false"),
		)

		t.Run("range of partitions", func(t *testing.T) {
			t.Parallel()
			suite.Run(
				t, template,
				integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
					vars.Var4 = "group" + testID
					require.NoError(t, createKafkaTopic(ctx, "localhost:"+kafkaPortStr, testID, 4))
				}),
				integration.StreamTestOptPort(kafkaPortStr),
				integration.StreamTestOptSleepAfterInput(time.Second*3),
				integration.StreamTestOptVarOne(":0-3"),
				integration.StreamTestOptVarTwo("1"),
				integration.StreamTestOptVarThree("false"),
			)
		})

		t.Run("checkpointed", func(t *testing.T) {
			t.Parallel()
			suiteSingleCheckpointedStream.Run(
				t, template,
				integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
					vars.Var4 = "group" + testID
					require.NoError(t, createKafkaTopic(ctx, "localhost:"+kafkaPortStr, testID, 1))
				}),
				integration.StreamTestOptPort(kafkaPortStr),
				integration.StreamTestOptSleepAfterInput(time.Second*3),
				integration.StreamTestOptVarOne(":0"),
				integration.StreamTestOptVarTwo("1000"),
				integration.StreamTestOptVarThree("false"),
			)
		})
	})

	t.Run("without consumer group", func(t *testing.T) {
		t.Parallel()
		suite.Run(
			t, template,
			integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
				require.NoError(t, createKafkaTopic(ctx, "localhost:"+kafkaPortStr, testID, 4))
			}),
			integration.StreamTestOptPort(kafkaPortStr),
			integration.StreamTestOptSleepAfterInput(time.Second*3),
			integration.StreamTestOptVarOne(":0-3"),
			integration.StreamTestOptVarTwo("1"),
			integration.StreamTestOptVarThree("false"),
		)
	})

	templateManualPartitioner := `
output:
  kafka:
    addresses: [ localhost:$PORT ]
    topic: topic-$ID
    max_in_flight: $MAX_IN_FLIGHT
    retry_as_batch: $VAR3
    metadata:
      exclude_prefixes: [ $OUTPUT_META_EXCLUDE_PREFIX ]
    batching:
      count: $OUTPUT_BATCH_COUNT
    partitioner: manual
    partition: '${! random_int() % 4 }'

input:
  kafka:
    addresses: [ localhost:$PORT ]
    topics: [ topic-$ID$VAR1 ]
    consumer_group: "$VAR4"
    checkpoint_limit: $VAR2
    start_from_oldest: true
    batching:
      count: $INPUT_BATCH_COUNT
`

	t.Run("manual_partitioner", func(t *testing.T) {
		t.Parallel()
		suite.Run(
			t, templateManualPartitioner,
			integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
				vars.Var4 = "group" + testID
				require.NoError(t, createKafkaTopic(ctx, "localhost:"+kafkaPortStr, testID, 4))
			}),
			integration.StreamTestOptPort(kafkaPortStr),
			integration.StreamTestOptVarTwo("1"),
			integration.StreamTestOptVarThree("false"),
		)
	})
}

func TestIntegrationSaramaOld(t *testing.T) {
	integration.CheckSkip(t)
	if runtime.GOOS == "darwin" {
		t.Skip("skipping test on macos")
	}

	t.Parallel()

	pool, err := dockertest.NewPool("")
	require.NoError(t, err)

	pool.MaxWait = time.Minute

	networks, _ := pool.Client.ListNetworks()
	hostIP := ""
	for _, network := range networks {
		if network.Name == "bridge" {
			hostIP = network.IPAM.Config[0].Gateway
		}
	}

	zkResource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "wurstmeister/zookeeper",
		Tag:        "latest",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, pool.Purge(zkResource))
	})
	_ = zkResource.Expire(900)
	zkAddr := fmt.Sprintf("%v:2181", zkResource.Container.NetworkSettings.IPAddress)

	kafkaPort, err := integration.GetFreePort()
	require.NoError(t, err)

	kafkaPortStr := strconv.Itoa(kafkaPort)
	env := []string{
		"KAFKA_ADVERTISED_HOST_NAME=" + hostIP,
		"KAFKA_BROKER_ID=1",
		"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=OUTSIDE:PLAINTEXT,INSIDE:PLAINTEXT",
		"KAFKA_LISTENERS=OUTSIDE://:" + kafkaPortStr + ",INSIDE://:9092",
		"KAFKA_ADVERTISED_LISTENERS=OUTSIDE://" + hostIP + ":" + kafkaPortStr + ",INSIDE://:9092",
		"KAFKA_INTER_BROKER_LISTENER_NAME=INSIDE",
		"KAFKA_ZOOKEEPER_CONNECT=" + zkAddr,
	}

	kafkaResource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository:   "wurstmeister/kafka",
		Tag:          "latest",
		ExposedPorts: []string{kafkaPortStr + "/tcp"},
		PortBindings: map[docker.Port][]docker.PortBinding{
			docker.Port(kafkaPortStr + "/tcp"): {{HostIP: "", HostPort: kafkaPortStr}},
		},
		Env: env,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, pool.Purge(kafkaResource))
	})
	_ = kafkaResource.Expire(900)

	address := fmt.Sprintf("%v:%v", hostIP, kafkaPortStr)

	require.NoError(t, pool.Retry(func() error {
		outConf := output.NewKafkaConfig()
		outConf.TargetVersion = "2.1.0"
		outConf.Addresses = []string{address}
		outConf.Topic = "pls_ignore_just_testing_connection"
		tmpOutput, serr := kafka.NewKafkaWriter(outConf, mock.NewManager())
		if serr != nil {
			return serr
		}
		defer tmpOutput.Close(context.Background())
		if serr := tmpOutput.Connect(context.Background()); serr != nil {
			return serr
		}
		return tmpOutput.WriteBatch(context.Background(), message.QuickBatch([][]byte{
			[]byte("foo message"),
		}))
	}))

	template := fmt.Sprintf(`
output:
  kafka:
    addresses: [ %v ]
    topic: topic-$ID
    max_in_flight: $MAX_IN_FLIGHT
    retry_as_batch: $VAR3
    batching:
      count: $OUTPUT_BATCH_COUNT

input:
  kafka:
    addresses: [ %v ]
    topics: [ topic-$ID$VAR1 ]
    consumer_group: consumer-$ID
    checkpoint_limit: $VAR2
    batching:
      count: $INPUT_BATCH_COUNT
`, address, address)

	suite := integration.StreamTests(
		integration.StreamTestOpenClose(),
		integration.StreamTestMetadata(),
		integration.StreamTestSendBatch(10),
		integration.StreamTestStreamSequential(1000),
		integration.StreamTestStreamParallel(1000),
		integration.StreamTestStreamParallelLossy(1000),
		integration.StreamTestSendBatchCount(10),
	)
	// In some tests include testing input level batching
	var suiteExt integration.StreamTestList
	suiteExt = append(suiteExt, suite...)
	suiteExt = append(suiteExt, integration.StreamTestReceiveBatchCount(10))

	// Only for checkpointed tests
	var suiteSingleCheckpointedStream integration.StreamTestList
	suiteSingleCheckpointedStream = append(suiteSingleCheckpointedStream, suite...)
	suiteSingleCheckpointedStream = append(suiteSingleCheckpointedStream, integration.StreamTestCheckpointCapture())

	t.Run("balanced", func(t *testing.T) {
		t.Parallel()
		suiteExt.Run(
			t, template,
			integration.StreamTestOptVarOne(""),
			integration.StreamTestOptVarTwo("1"),
			integration.StreamTestOptVarThree("false"),
		)

		t.Run("checkpointed", func(t *testing.T) {
			t.Parallel()
			suiteSingleCheckpointedStream.Run(
				t, template,
				integration.StreamTestOptVarOne(""),
				integration.StreamTestOptVarTwo("1000"),
				integration.StreamTestOptVarThree("false"),
			)
		})

		t.Run("retry as batch", func(t *testing.T) {
			t.Parallel()
			suiteExt.Run(
				t, template,
				integration.StreamTestOptVarOne(""),
				integration.StreamTestOptVarTwo("1"),
				integration.StreamTestOptVarThree("true"),
			)
		})

		t.Run("with four partitions", func(t *testing.T) {
			t.Parallel()
			suite.Run(
				t, template,
				integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
					require.NoError(t, createKafkaTopic(ctx, address, testID, 4))
				}),
				integration.StreamTestOptVarOne(""),
				integration.StreamTestOptVarTwo("1"),
				integration.StreamTestOptVarThree("false"),
			)

			t.Run("checkpointed", func(t *testing.T) {
				t.Parallel()
				suite.Run(
					t, template,
					integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
						require.NoError(t, createKafkaTopic(ctx, address, testID, 4))
					}),
					integration.StreamTestOptVarOne(""),
					integration.StreamTestOptVarTwo("1000"),
					integration.StreamTestOptVarThree("false"),
				)
			})
		})
	})

	t.Run("partitions", func(t *testing.T) {
		t.Parallel()
		suiteExt.Run(
			t, template,
			integration.StreamTestOptVarOne(":0"),
			integration.StreamTestOptVarTwo("1"),
			integration.StreamTestOptVarThree("false"),
		)

		t.Run("checkpointed", func(t *testing.T) {
			t.Parallel()
			suiteSingleCheckpointedStream.Run(
				t, template,
				integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
					require.NoError(t, createKafkaTopic(ctx, "localhost:"+kafkaPortStr, testID, 1))
				}),
				integration.StreamTestOptVarOne(":0"),
				integration.StreamTestOptVarTwo("1000"),
				integration.StreamTestOptVarThree("false"),
			)
		})

		t.Run("with four partitions", func(t *testing.T) {
			t.Parallel()
			suite.Run(
				t, template,
				integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
					topicName := "topic-" + testID
					vars.Var1 = fmt.Sprintf(":0,%v:1,%v:2,%v:3", topicName, topicName, topicName)
					require.NoError(t, createKafkaTopic(ctx, address, testID, 4))
				}),
				integration.StreamTestOptSleepAfterInput(time.Second*3),
				integration.StreamTestOptVarTwo("1"),
				integration.StreamTestOptVarThree("false"),
			)

			t.Run("checkpointed", func(t *testing.T) {
				t.Parallel()
				suite.Run(
					t, template,
					integration.StreamTestOptPreTest(func(t testing.TB, ctx context.Context, testID string, vars *integration.StreamTestConfigVars) {
						topicName := "topic-" + testID
						vars.Var1 = fmt.Sprintf(":0,%v:1,%v:2,%v:3", topicName, topicName, topicName)
						require.NoError(t, createKafkaTopic(ctx, address, testID, 4))
					}),
					integration.StreamTestOptSleepAfterInput(time.Second*3),
					integration.StreamTestOptVarTwo("1000"),
					integration.StreamTestOptVarThree("false"),
				)
			})
		})
	})
}
