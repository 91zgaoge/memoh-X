-- Add kimi-coding to client_type check constraint
-- First, drop the existing constraint
ALTER TABLE llm_providers DROP CONSTRAINT IF EXISTS llm_providers_client_type_check;

-- Add the new constraint with kimi-coding included
ALTER TABLE llm_providers ADD CONSTRAINT llm_providers_client_type_check
CHECK (client_type = ANY (ARRAY[
    'openai'::text, 'openai-compat'::text, 'anthropic'::text, 'google'::text,
    'azure'::text, 'bedrock'::text, 'mistral'::text, 'xai'::text, 'ollama'::text,
    'dashscope'::text, 'deepseek'::text, 'zai-global'::text, 'zai-cn'::text,
    'zai-coding-global'::text, 'zai-coding-cn'::text, 'minimax-global'::text,
    'minimax-cn'::text, 'moonshot-global'::text, 'moonshot-cn'::text,
    'volcengine'::text, 'volcengine-coding'::text, 'qianfan'::text, 'groq'::text,
    'openrouter'::text, 'together'::text, 'fireworks'::text, 'perplexity'::text,
    'zhipu'::text, 'siliconflow'::text, 'nvidia'::text, 'bailing'::text,
    'xiaomi'::text, 'longcat'::text, 'modelscope'::text, 'kimi-coding'::text
]));
