-- name: GetSettingsByBotID :one
SELECT
  bots.id AS bot_id,
  bots.max_context_load_time,
  bots.language,
  bots.allow_guest,
  bots.group_require_mention,
  chat_models.model_id AS chat_model_id,
  memory_models.model_id AS memory_model_id,
  embedding_models.model_id AS embedding_model_id,
  vlm_models.model_id AS vlm_model_id,
  background_models.model_id AS background_model_id,
  image_models.model_id AS image_model_id,
  search_providers.id AS search_provider_id
FROM bots
LEFT JOIN models AS chat_models ON chat_models.id = bots.chat_model_id
LEFT JOIN models AS memory_models ON memory_models.id = bots.memory_model_id
LEFT JOIN models AS embedding_models ON embedding_models.id = bots.embedding_model_id
LEFT JOIN models AS vlm_models ON vlm_models.id = bots.vlm_model_id
LEFT JOIN models AS background_models ON background_models.id = bots.background_model_id
LEFT JOIN models AS image_models ON image_models.id = bots.image_model_id
LEFT JOIN search_providers ON search_providers.id = bots.search_provider_id
WHERE bots.id = $1;

-- name: UpsertBotSettings :one
WITH params AS (
  SELECT
    sqlc.arg(id)::uuid as bot_id,
    sqlc.arg(max_context_load_time)::int as max_context_load_time,
    sqlc.arg(language)::text as language,
    sqlc.arg(allow_guest)::bool as allow_guest,
    sqlc.arg(group_require_mention)::bool as group_require_mention,
    sqlc.narg(chat_model_id)::uuid as chat_model_id,
    sqlc.narg(memory_model_id)::uuid as memory_model_id,
    sqlc.narg(embedding_model_id)::uuid as embedding_model_id,
    sqlc.narg(vlm_model_id)::uuid as vlm_model_id,
    sqlc.narg(background_model_id)::uuid as background_model_id,
    sqlc.narg(image_model_id)::uuid as image_model_id,
    sqlc.narg(search_provider_id)::uuid as search_provider_id
),
updated AS (
  UPDATE bots
  SET max_context_load_time = params.max_context_load_time,
      language = params.language,
      allow_guest = params.allow_guest,
      group_require_mention = params.group_require_mention,
      chat_model_id = COALESCE(params.chat_model_id, bots.chat_model_id),
      memory_model_id = COALESCE(params.memory_model_id, bots.memory_model_id),
      embedding_model_id = COALESCE(params.embedding_model_id, bots.embedding_model_id),
      vlm_model_id = CASE
          WHEN params.vlm_model_id IS NULL THEN bots.vlm_model_id
          WHEN params.vlm_model_id = '00000000-0000-0000-0000-000000000000'::uuid THEN NULL
          ELSE params.vlm_model_id
      END,
      background_model_id = CASE
          WHEN params.background_model_id IS NULL THEN bots.background_model_id
          WHEN params.background_model_id = '00000000-0000-0000-0000-000000000000'::uuid THEN NULL
          ELSE params.background_model_id
      END,
      image_model_id = CASE
          WHEN params.image_model_id IS NULL THEN bots.image_model_id
          WHEN params.image_model_id = '00000000-0000-0000-0000-000000000000'::uuid THEN NULL
          ELSE params.image_model_id
      END,
      search_provider_id = COALESCE(params.search_provider_id, bots.search_provider_id),
      updated_at = now()
  FROM params
  WHERE bots.id = params.bot_id
  RETURNING bots.id, bots.max_context_load_time, bots.language, bots.allow_guest, bots.group_require_mention, bots.chat_model_id, bots.memory_model_id, bots.embedding_model_id, bots.vlm_model_id, bots.background_model_id, bots.image_model_id, bots.search_provider_id
)
SELECT
  updated.id AS bot_id,
  updated.max_context_load_time,
  updated.language,
  updated.allow_guest,
  updated.group_require_mention,
  chat_models.model_id AS chat_model_id,
  memory_models.model_id AS memory_model_id,
  embedding_models.model_id AS embedding_model_id,
  vlm_models.model_id AS vlm_model_id,
  background_models.model_id AS background_model_id,
  image_models.model_id AS image_model_id,
  search_providers.id AS search_provider_id
FROM updated
LEFT JOIN models AS chat_models ON chat_models.id = updated.chat_model_id
LEFT JOIN models AS memory_models ON memory_models.id = updated.memory_model_id
LEFT JOIN models AS embedding_models ON embedding_models.id = updated.embedding_model_id
LEFT JOIN models AS vlm_models ON vlm_models.id = updated.vlm_model_id
LEFT JOIN models AS background_models ON background_models.id = updated.background_model_id
LEFT JOIN models AS image_models ON image_models.id = updated.image_model_id
LEFT JOIN search_providers ON search_providers.id = updated.search_provider_id;

-- name: DeleteSettingsByBotID :exec
UPDATE bots
SET max_context_load_time = 1440,
    language = 'auto',
    allow_guest = false,
    group_require_mention = true,
    chat_model_id = NULL,
    memory_model_id = NULL,
    embedding_model_id = NULL,
    vlm_model_id = NULL,
    background_model_id = NULL,
    image_model_id = NULL,
    search_provider_id = NULL,
    updated_at = now()
WHERE id = $1;
