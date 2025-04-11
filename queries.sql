-- sleeps for 3 seconds
SELECT sleep(3) SETTINGS max_concurrent_queries_for_all_users=1,max_execution_time=1