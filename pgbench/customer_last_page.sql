-- pgbench script: read the customer table with 1000-row pages, ordered by name,
-- always fetching the last page (worst case for OFFSET-based pagination).
\set page_size 1000

SELECT count(*) AS cnt FROM customer \gset
\set page_offset ((:cnt - 1) / :page_size) * :page_size

SELECT * FROM customer ORDER BY c_name LIMIT :page_size OFFSET :page_offset;
