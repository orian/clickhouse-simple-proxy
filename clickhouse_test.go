package main

import (
	"context"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/stretchr/testify/assert"
)

// curl 'http://localhost:8123/?add_http_cors_header=1&default_format=JSONEachRow&user=default&password=clickhouse' -X POST -H 'User-Agent: Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:138.0) Gecko/20100101 Firefox/138.0' -H 'Accept: */*' -H 'Accept-Language: en-US,en;q=0.5' -H 'Accept-Encoding: gzip, deflate, br, zstd' -H 'Referer: http://localhost:8123/play' -H 'Authorization: never' -H 'Content-Type: text/plain;charset=UTF-8' -H 'Origin: http://localhost:8123' -H 'DNT: 1' -H 'Connection: keep-alive' -H 'Cookie: ph_fake token_posthog=%7B%22distinct_id%22%3A%22019270dc-bdc4-76de-b7f9-b23b8c70ee43%22%2C%22%24sesid%22%3A%5B1728470367683%2C%22019270dc-bdc3-7086-a290-1895d5fc0e92%22%2C1728470367683%5D%7D; ph_phc_lYC0UiOJMcaPDaNkpUJ2DgvxSqUzo1frjId91LlyMtS_posthog=%7B%22distinct_id%22%3A%22Vc0Zfa7FCJYWwE4pTmhZaGWP9tC4BROKqPs7x8pawrz%22%2C%22%24sesid%22%3A%5B1741269614677%2C%2201956b96-0692-739e-b897-c2088898d3cb%22%2C1741266749074%5D%2C%22%24epp%22%3Atrue%2C%22%24initial_person_info%22%3A%7B%22r%22%3A%22%24direct%22%2C%22u%22%3A%22http%3A%2F%2Flocalhost%3A8010%2Flogin%3Fnext%3D%2F%22%7D%7D; csrftoken=Qy46jnjxyMgapQpaP5SPqT3Vkj4qdS7g' -H 'Sec-Fetch-Dest: empty' -H 'Sec-Fetch-Mode: cors' -H 'Sec-Fetch-Site: same-origin' -H 'Priority: u=0' --data-raw 'SELECT version() AS v, uptime() AS t'
//
//	await fetch("http://localhost:8123/?add_http_cors_header=1&default_format=JSONEachRow&user=default&password=clickhouse", {
//		   "credentials": "include",
//		   "headers": {
//		       "User-Agent": "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:138.0) Gecko/20100101 Firefox/138.0",
//		       "Accept": "*/*",
//		       "Accept-Language": "en-US,en;q=0.5",
//		       "Authorization": "never",
//		       "Content-Type": "text/plain;charset=UTF-8",
//		       "Sec-Fetch-Dest": "empty",
//		       "Sec-Fetch-Mode": "cors",
//		       "Sec-Fetch-Site": "same-origin",
//		       "Priority": "u=0"
//		   },
//		   "referrer": "http://localhost:8123/play",
//		   "body": "SELECT version() AS v, uptime() AS t",
//		   "method": "POST",
//		   "mode": "cors"
//		});
func TestClickHouseQueriesRawHTTP(t *testing.T) {
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	c := http.Client{
		Jar:     jar,
		Timeout: 0,
	}

	u := url.URL{
		Scheme: "http",
		Host:   "localhost:8123",
		Path:   "/",
	}
	vals := url.Values{
		"add_http_cors_header": {"1"},
		"default_format":       {"JSONEachRow"},
		"user":                 {"default"},
		"password":             {"clickhouse"},
	}
	u.RawQuery = vals.Encode()
	queryUrl := u.String()
	getResponse := func(query string) string {
		req, err := http.NewRequest(http.MethodPost, queryUrl,
			strings.NewReader(query))
		require.NoError(t, err)
		r, err := c.Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, r.Body.Close())
		if r.StatusCode != http.StatusOK {
			t.Log(r.Status)
			t.Log(string(body))
			require.Equal(t, http.StatusOK, r.StatusCode)
		}

		return string(body)
	}

	t.Logf("get version: %s", getResponse("SELECT version() AS v, uptime() AS t"))
	t.Logf("create table: %s", getResponse(`
		CREATE TABLE IF NOT EXISTS test_table (
			id UInt32,
			name String,
			created_at DateTime
		) ENGINE = MergeTree()
		ORDER BY id
	`))
}

// Example error from CH
// {"exception": "Code: 57. DB::Exception: Table default.test_table already exists. (TABLE_ALREADY_EXISTS) (version 25.5.1.2782 (official build))"}

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
	ctx = context.WithValue(ctx, &customHeadersKey, customHeaders)
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
