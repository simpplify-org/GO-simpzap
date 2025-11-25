-- name: UpsertDevice :one
INSERT INTO whatsapp_device (number, container_id, endpoint, version, updated_who)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (number)
DO UPDATE SET
    container_id = EXCLUDED.container_id,
    endpoint = EXCLUDED.endpoint,
    version = EXCLUDED.version,
    updated_who = EXCLUDED.updated_who,
    updated_at = NOW(),
    active = true
RETURNING id;

-- name: SoftDeleteDevice :exec
UPDATE whatsapp_device
SET active = FALSE
WHERE number = $1 AND active = TRUE;

-- name: GetDevice :one
SELECT * FROM whatsapp_device
WHERE number = $1 AND active = TRUE;

-- name: GetDevices :many
SELECT * FROM whatsapp_device
WHERE active = TRUE;

