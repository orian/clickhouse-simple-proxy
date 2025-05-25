package main

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/stretchr/testify/assert"
)

func TestClickHouseQueries(t *testing.T) {
	// Create a connection to ClickHouse using HTTP protocol
	conn := clickhouse.OpenDB(&clickhouse.Options{
		Addr: []string{"localhost:18123"},
		Auth: clickhouse.Auth{
			Database: "default",
			Username: "default",
			Password: "clickhouse",
		},
		Protocol: clickhouse.HTTP,
	})
	require.NotNil(t, conn)

	//conn, err := clickhouse.Open(&clickhouse.Options{
	//	Addr: []string{"localhost:8123"},
	//	Auth: clickhouse.Auth{
	//		Database: "default",
	//		Username: "default",
	//		Password: "clickhouse",
	//	},
	//	Settings: clickhouse.Settings{
	//		"max_execution_time": 60,
	//	},
	//	DialTimeout:          time.Second * 10,
	//	MaxOpenConns:         5,
	//	MaxIdleConns:         5,
	//	ConnMaxLifetime:      time.Hour,
	//	ConnOpenStrategy:     clickhouse.ConnOpenInOrder,
	//	BlockBufferSize:      10,
	//	MaxCompressionBuffer: 10240,
	//	Protocol:             clickhouse.HTTP,
	//})
	//assert.NoError(t, err)

	// Ensure connection is closed after the test
	defer func() {
		err := conn.Close()
		assert.NoError(t, err)
	}()

	// First query: Create a test table
	ctx := context.Background()
	var customHeadersKey string = "custom headers key"
	customHeaders := map[string]string{
		"X-User-Id": "1234",
	}
	ctx = context.WithValue(ctx, clickhouse.Context(), customHeaders)
	_, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS test_table (
			id UInt32,
			name String,
			created_at DateTime
		) ENGINE = MergeTree()
		ORDER BY id
	`)
	require.NoError(t, err)

	// Insert some test data
	_, err = conn.ExecContext(ctx, "INSERT INTO test_table (id, name, created_at) VALUES (?, ?, ?)",
		uint32(1),
		"Test 1",
		time.Now(),
	)
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, "INSERT INTO test_table (id, name, created_at) VALUES (?, ?, ?)",
		uint32(2),
		"Test 2",
		time.Now(),
	)
	require.NoError(t, err)
	// Second query: Select and verify the data
	rows, err := conn.QueryContext(context.Background(), "SELECT id, name FROM test_table ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()

	// Verify the results
	var (
		id   uint32
		name string
	)

	if rows.Err() != nil {
		t.Fatalf("rows.Err() returned an error: %v", rows.Err())
	}

	// Check first row
	assert.True(t, rows.Next())
	err = rows.Scan(&id, &name)
	assert.NoError(t, err)
	assert.Equal(t, uint32(1), id)
	assert.Equal(t, "Test 1", name)

	// Check second row
	assert.True(t, rows.Next())
	err = rows.Scan(&id, &name)
	assert.NoError(t, err)
	assert.Equal(t, uint32(2), id)
	assert.Equal(t, "Test 2", name)

	// Ensure no more rows
	assert.False(t, rows.Next())

	// Clean up: Drop the test table
	_, err = conn.Exec("DROP TABLE IF EXISTS test_table")
	assert.NoError(t, err)
}
