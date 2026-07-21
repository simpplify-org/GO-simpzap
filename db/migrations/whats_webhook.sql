CREATE TABLE whats_webhook (
    id SERIAL PRIMARY KEY,
    number VARCHAR(20) NOT NULL,
    message TEXT NOT NULL,
    callback_url TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);


CREATE OR REPLACE FUNCTION "FUN_notify_whatsapp_webhook"() RETURNS trigger AS $$
DECLARE
payload JSON;
BEGIN
    payload = json_build_object(
        'id', NEW.id,
        'number', NEW.number,
        'message', NEW.message,
        'callback_url', NEW.callback_url,
        'created_at', NEW.created_at
    );

    PERFORM pg_notify('whatsapp_webhook_channel', payload::text);
RETURN NEW;
END;
$$ LANGUAGE plpgsql;


CREATE TRIGGER "TRI_whatsapp_webhook_notify"
AFTER INSERT ON whats_webhook
FOR EACH ROW
EXECUTE FUNCTION "FUN_notify_whatsapp_webhook";
