#!/bin/bash

# ClickHouse Cluster Test Script
# This script tests the setup of a ClickHouse cluster with two nodes and ZooKeeper

# Configuration
CH_HOST1="localhost"
CH_PORT1="8123"
CH_HOST2="localhost"
CH_PORT2="8124"
CH_USER="default"
CH_PASSWORD="clickhouse"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Function to make HTTP requests to ClickHouse
function ch_query() {
    local host=$1
    local port=$2
    local query=$3
    local method=${4:-"GET"}
    
    if [ "$method" = "POST" ]; then
        curl -s -X POST "http://${host}:${port}/?user=${CH_USER}&password=${CH_PASSWORD}" -d "$query"
    else
        curl -s "http://${host}:${port}/?query=${query}&user=${CH_USER}&password=${CH_PASSWORD}"
    fi
}

# Function to print section headers
function print_header() {
    echo -e "\n${YELLOW}=== $1 ===${NC}"
}

# Function to check if a command succeeded
function check_result() {
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Success${NC}"
    else
        echo -e "${RED}✗ Failed${NC}"
        exit 1
    fi
}

# Wait for services to be ready
echo "Waiting for ClickHouse services to be ready..."
sleep 5

# 1. Test Basic Connectivity to Node 1
print_header "Testing connectivity to Node 1"
result=$(ch_query "$CH_HOST1" "$CH_PORT1" "SELECT%201")
if [ "$result" = "1" ]; then
    echo -e "${GREEN}✓ Node 1 is accessible${NC}"
else
    echo -e "${RED}✗ Node 1 is not accessible${NC}"
    exit 1
fi

# 2. Test Basic Connectivity to Node 2
print_header "Testing connectivity to Node 2"
result=$(ch_query "$CH_HOST2" "$CH_PORT2" "SELECT%201")
if [ "$result" = "1" ]; then
    echo -e "${GREEN}✓ Node 2 is accessible${NC}"
else
    echo -e "${RED}✗ Node 2 is not accessible${NC}"
    exit 1
fi

# 3. Create a Table on the Cluster
print_header "Creating test table on the cluster"
ch_query "$CH_HOST1" "$CH_PORT1" "CREATE TABLE test_table ON CLUSTER test_cluster (id UInt32, value String) ENGINE = MergeTree ORDER BY id" "POST"
check_result

# 4. Create a Distributed Table
print_header "Creating distributed table"
ch_query "$CH_HOST1" "$CH_PORT1" "CREATE TABLE test_table_distributed ON CLUSTER test_cluster (id UInt32, value String) ENGINE = Distributed(test_cluster, default, test_table, rand())" "POST"
check_result

# 5. Insert Data into the Distributed Table
print_header "Inserting test data"
ch_query "$CH_HOST1" "$CH_PORT1" "INSERT INTO test_table_distributed VALUES (1, 'test1'), (2, 'test2'), (3, 'test3')" "POST"
check_result

# 6. Query Data from the Distributed Table
print_header "Querying data from distributed table"
echo "Data in distributed table:"
ch_query "$CH_HOST1" "$CH_PORT1" "SELECT * FROM test_table_distributed ORDER BY id FORMAT TabSeparated" "POST"
check_result

# 7. Check Cluster Status
print_header "Checking cluster status"
echo "Cluster configuration:"
ch_query "$CH_HOST1" "$CH_PORT1" "SELECT * FROM system.clusters WHERE cluster = 'test_cluster'" "POST"
check_result

# 8. Check Table Distribution
print_header "Checking data distribution across nodes"
echo "Data distribution:"
ch_query "$CH_HOST1" "$CH_PORT1" "SELECT hostName(), count() FROM test_table GROUP BY hostName()" "POST"
check_result

# 9. Test Query from Second Node
print_header "Testing query from second node"
echo "Data from Node 2:"
ch_query "$CH_HOST2" "$CH_PORT2" "SELECT * FROM test_table_distributed ORDER BY id FORMAT TabSeparated" "POST"
check_result

# 10. Check ZooKeeper Connection
print_header "Checking ZooKeeper connection"
echo "ZooKeeper root nodes:"
ch_query "$CH_HOST1" "$CH_PORT1" "SELECT * FROM system.zookeeper WHERE path = '/'" "POST"
check_result

# Clean up test tables
print_header "Cleaning up test tables"
ch_query "$CH_HOST1" "$CH_PORT1" "DROP TABLE test_table_distributed ON CLUSTER test_cluster" "POST"
ch_query "$CH_HOST1" "$CH_PORT1" "DROP TABLE test_table ON CLUSTER test_cluster" "POST"
check_result

echo -e "\n${GREEN}All tests completed successfully!${NC}"
echo -e "${GREEN}Your ClickHouse cluster is properly configured and working.${NC}" 
