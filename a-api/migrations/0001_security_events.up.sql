-- Security events (ADR-0020): one MergeTree table, partitioned by day and ordered so that
-- tenant-scoped, zone-scoped and time-bounded reads skip almost everything. Retention is the
-- table's TTL. {{database}} and {{retention_days}} are filled from ANALYTICS_DATABASE and
-- ANALYTICS_RETENTION_DAYS.
CREATE DATABASE IF NOT EXISTS {{database}}

-- migrate:statement-break
CREATE TABLE IF NOT EXISTS {{database}}.security_events (
	tenant_id         UUID,
	id                UUID,
	zone_id           String,
	triggered_by      LowCardinality(String),
	triggered_by_id   String,
	triggered_by_name String,
	action            LowCardinality(String),
	source_ip         String,
	request_path      String,
	occurred_at       DateTime64(3, 'UTC'),
	ingested_at       DateTime64(3, 'UTC')
) ENGINE = MergeTree
PARTITION BY toYYYYMMDD(occurred_at)
ORDER BY (tenant_id, zone_id, occurred_at, id)
TTL toDateTime(occurred_at) + INTERVAL {{retention_days}} DAY
