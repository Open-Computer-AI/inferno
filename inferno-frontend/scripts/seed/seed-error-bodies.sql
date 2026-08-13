-- Response bodies for ops_error_logs.
--
-- WHY THIS EXISTS
-- Every other ops seed produces error rows with error_body and
-- upstream_error_detail both NULL, so OpsErrorDetailModal's "Response" block
-- renders the literal string "N/A" on all 458 rows. A <pre> that only ever
-- shows two characters cannot be verified: you learn nothing about its
-- max-height, its overflow, its wrapping, or whether long unbroken JSON pushes
-- the dialog sideways.
--
-- Bodies are shaped like the real thing each upstream actually returns, and
-- deliberately vary in size: a short one, a normal one, and one long enough to
-- exceed the 460px max-height so the scroll path gets exercised.
--
-- Idempotent: re-running overwrites the same rows with the same values.

-- 5xx from Anthropic: overloaded. Short body.
UPDATE ops_error_logs
SET error_body = '{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}'
WHERE status_code = 529
  AND id IN (SELECT id FROM ops_error_logs WHERE status_code = 529 ORDER BY created_at DESC LIMIT 25);

-- 429 rate limit, with the headers an operator actually wants to see.
UPDATE ops_error_logs
SET error_body = '{"type":"error","error":{"type":"rate_limit_error","message":"Number of request tokens has exceeded your per-minute rate limit"},"request_id":"req_011CQ8xKm2vZ","retry_after":"27","rate_limit":{"requests_limit":4000,"requests_remaining":0,"tokens_limit":400000,"tokens_remaining":0,"reset":"2026-08-13T12:41:00Z"}}'
WHERE status_code = 429
  AND id IN (SELECT id FROM ops_error_logs WHERE status_code = 429 ORDER BY created_at DESC LIMIT 25);

-- 400 client error: a validation failure naming the offending field.
UPDATE ops_error_logs
SET error_body = '{"type":"error","error":{"type":"invalid_request_error","message":"messages.3.content.0.text: Field required"}}'
WHERE status_code = 400
  AND id IN (SELECT id FROM ops_error_logs WHERE status_code = 400 ORDER BY created_at DESC LIMIT 25);

-- 500: long enough to overflow the 460px max-height and force a scroll, and it
-- carries a stack trace, which is the case where an unbroken token could push
-- the dialog sideways if the <pre> did not scroll on its own.
UPDATE ops_error_logs
SET error_body = '{"type":"error","error":{"type":"api_error","message":"Internal server error while proxying upstream request"},"gateway":{"request_id":"gw_7f3a91c2e845bd06","attempt":3,"attempts":[{"n":1,"account":"seed-account","status":529,"latency_ms":1841,"error":"overloaded_error"},{"n":2,"account":"seed-account-b","status":503,"latency_ms":2204,"error":"upstream_unavailable"},{"n":3,"account":"seed-account-c","status":500,"latency_ms":1990,"error":"api_error"}],"route":{"requested_model":"claude-sonnet-4-5","resolved_model":"claude-sonnet-4-5-20250929","group":"default","platform":"anthropic"},"trace":["at UpstreamClient.send (/srv/gateway/upstream/client.go:214)","at RetryPolicy.execute (/srv/gateway/retry/policy.go:88)","at ProxyHandler.ServeHTTP (/srv/gateway/proxy/handler.go:301)","at http.serverHandler.ServeHTTP (/usr/local/go/src/net/http/server.go:2938)","at http.(*conn).serve (/usr/local/go/src/net/http/server.go:2009)"],"timings":{"auth_ms":4,"routing_ms":11,"upstream_ms":6035,"response_ms":2}}}'
WHERE status_code = 500
  AND id IN (SELECT id FROM ops_error_logs WHERE status_code = 500 ORDER BY created_at DESC LIMIT 25);

-- 503 gets upstream_error_detail rather than error_body, which is the other
-- branch resolvePrimaryResponseBody() can take.
UPDATE ops_error_logs
SET upstream_error_detail = '{"error":{"code":"service_unavailable","message":"The upstream provider is temporarily unavailable. Retry with backoff.","param":null,"type":"server_error"}}'
WHERE status_code = 503
  AND id IN (SELECT id FROM ops_error_logs WHERE status_code = 503 ORDER BY created_at DESC LIMIT 25);

-- Not valid JSON: prettyJSON() falls through its catch and prints raw. An HTML
-- error page from a load balancer is the usual way this happens in production.
UPDATE ops_error_logs
SET error_body = '<html><head><title>502 Bad Gateway</title></head><body><center><h1>502 Bad Gateway</h1></center><hr><center>nginx/1.24.0</center></body></html>'
WHERE id IN (SELECT id FROM ops_error_logs WHERE status_code = 503 ORDER BY created_at ASC LIMIT 5);

SELECT status_code,
       count(*) FILTER (WHERE coalesce(error_body, '') <> '') AS with_body,
       count(*) FILTER (WHERE coalesce(upstream_error_detail, '') <> '') AS with_upstream_detail,
       count(*) AS total
FROM ops_error_logs
GROUP BY 1
ORDER BY 1;
