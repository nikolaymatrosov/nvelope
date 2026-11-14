-- Fixed-window throttle counters (internal/platform/ydbthrottle), the
-- serverless replacement for the Redis limiters. One row per key per window;
-- TTL deletes a row once its window has ended.
CREATE TABLE IF NOT EXISTS throttle_windows (
    throttle_key Utf8      NOT NULL,
    window_start Timestamp NOT NULL,
    hits         Uint64    NOT NULL,
    expire_at    Timestamp NOT NULL,
    PRIMARY KEY (throttle_key, window_start)
) WITH (TTL = Interval("PT0S") ON expire_at);
