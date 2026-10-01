-- Volume mensal de mensagens por device, para estimar o custo na API oficial
-- (WhatsApp Cloud API da Meta).
--
-- Na cobrança por mensagem da Meta, responder dentro da janela de 24h aberta
-- pela última mensagem do usuário (janela de atendimento) é grátis; fora dela
-- só é possível enviar template, e cada template é cobrado. Por isso as
-- mensagens enviadas são separadas em "dentro_janela_24h" (grátis) e
-- "fora_janela_24h" (cada uma vira, aproximadamente, um template cobrado).
--
-- Grupos e reações ficam de fora (a API oficial não cobra reação e o uso em
-- grupos é diferente).
WITH msgs AS (
    SELECT *
    FROM whatsapp_messages
    WHERE NOT is_group
      AND message_type <> 'reaction'
),
enviadas AS (
    SELECT m.device_phone,
           m.sent_at,
           EXISTS (
               SELECT 1
               FROM msgs r
               WHERE r.device_phone = m.device_phone
                 AND r.chat_jid = m.chat_jid
                 AND NOT r.from_me
                 AND r.sent_at <= m.sent_at
                 AND r.sent_at > m.sent_at - INTERVAL '24 hours'
           ) AS dentro_janela
    FROM msgs m
    WHERE m.from_me
)
SELECT date_trunc('month', x.sent_at AT TIME ZONE 'America/Sao_Paulo')::date AS mes,
       x.device_phone,
       COUNT(*) FILTER (WHERE x.tipo = 'recebida')     AS recebidas,
       COUNT(*) FILTER (WHERE x.tipo = 'dentro')       AS enviadas_dentro_janela_24h,
       COUNT(*) FILTER (WHERE x.tipo = 'fora')         AS enviadas_fora_janela_24h,
       COUNT(*)                                        AS total
FROM (
    SELECT device_phone, sent_at, 'recebida' AS tipo FROM msgs WHERE NOT from_me
    UNION ALL
    SELECT device_phone, sent_at, CASE WHEN dentro_janela THEN 'dentro' ELSE 'fora' END FROM enviadas
) x
GROUP BY 1, 2
ORDER BY 1, 2;

-- Contatos distintos por mês (útil para comparar com a cobrança antiga por
-- conversa, caso a proposta da Meta venha nesse formato).
SELECT date_trunc('month', sent_at AT TIME ZONE 'America/Sao_Paulo')::date AS mes,
       device_phone,
       COUNT(DISTINCT chat_jid) AS contatos_distintos
FROM whatsapp_messages
WHERE NOT is_group
GROUP BY 1, 2
ORDER BY 1, 2;
