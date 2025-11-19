-- name: InsertWebhook :exec
INSERT INTO whatsapp_device_webhook (device_id, number, phrase, url, url_method, active, body)
VALUES ($1, $2, $3, $4, $5, TRUE, $6)
ON CONFLICT (device_id, number, phrase, url, url_method, body) DO NOTHING;

-- name: ListWebhooksByDevice :many
SELECT *
FROM whatsapp_device_webhook
WHERE device_id = $1 AND active = TRUE
ORDER BY id;

-- name: GetWebhookByPhrase :one
SELECT *
FROM whatsapp_device_webhook
WHERE device_id = $1 
  AND number = $2 
  AND phrase = $3
  AND active = TRUE;

-- name: SoftDeleteWebhook :exec
UPDATE whatsapp_device_webhook
SET active = FALSE,
    updated_at = NOW()
WHERE id = $1;

