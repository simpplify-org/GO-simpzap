-- name: InsertEventLog :exec
INSERT INTO whatsapp_event_log (number, ip, method, endpoint, user_agent, status_code, request_body)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListLogsByNumber :many
SELECT *
FROM whatsapp_event_log
WHERE number = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListLastLogs :many
SELECT *
FROM whatsapp_event_log
ORDER BY created_at DESC
LIMIT $1;