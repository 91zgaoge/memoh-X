import { Elysia, sse } from 'elysia'
import { generateText } from 'ai'
import z from 'zod'
import { createAgent } from '../agent'
import { createAuthFetcher, getBaseUrl } from '../index'
import { createModel } from '../model'
import { ModelConfig, AgentParams, SYSTEM_SAFE_PROVIDERS } from '../types'
import { bearerMiddleware } from '../middlewares/bearer'
import { AgentSkillModel, AllowedActionModel, AttachmentModel, IdentityContextModel, MCPConnectionModel, ModelConfigModel, ScheduleModel } from '../models'
import { allActions } from '../types'

const AgentModel = z.object({
  model: ModelConfigModel,
  backgroundModel: ModelConfigModel.optional(),
  activeContextTime: z.number(),
  language: z.string().optional().default(''),
  timezone: z.string().optional().default('UTC'),
  channels: z.array(z.string()),
  currentChannel: z.string(),
  allowedActions: z.array(AllowedActionModel).optional().default(allActions),
  messages: z.array(z.any()),
  usableSkills: z.array(AgentSkillModel).optional().default([]),
  skills: z.array(z.string()),
  identity: IdentityContextModel,
  attachments: z.array(AttachmentModel).optional().default([]),
  mcpConnections: z.array(MCPConnectionModel).optional().default([]),
  botIdentity: z.string().optional().default(''),
  botSoul: z.string().optional().default(''),
  botTask: z.string().optional().default(''),
  allowSelfEvolution: z.boolean().optional().default(true),
})

type AgentBody = z.infer<typeof AgentModel>

function buildAgentParams(body: AgentBody, bearer: string): AgentParams {
  return {
    model: body.model as ModelConfig,
    backgroundModel: body.backgroundModel as ModelConfig | undefined,
    activeContextTime: body.activeContextTime,
    language: body.language || undefined,
    timezone: body.timezone || 'UTC',
    channels: body.channels,
    currentChannel: body.currentChannel,
    allowedActions: body.allowedActions,
    identity: body.identity,
    auth: {
      bearer,
      baseUrl: getBaseUrl(),
    },
    skills: body.usableSkills,
    mcpConnections: body.mcpConnections,
    botIdentity: body.botIdentity,
    botSoul: body.botSoul,
    botTask: body.botTask,
    allowSelfEvolution: body.allowSelfEvolution,
  }
}

export const chatModule = new Elysia({ prefix: '/chat' })
  .use(bearerMiddleware)
  .post('/', async ({ body, bearer }) => {
    const authFetcher = createAuthFetcher(bearer)
    const { ask } = createAgent(buildAgentParams(body, bearer!), authFetcher)
    return ask({
      query: body.query,
      messages: body.messages,
      skills: body.skills,
      attachments: body.attachments,
    })
  }, {
    body: AgentModel.extend({
      query: z.string(),
    }),
  })
  .post('/stream', async function* ({ body, bearer }) {
    try {
      console.log('[Agent Stream] Received request:', { query: body.query?.slice(0, 200), messageCount: body.messages?.length, attachmentCount: body.attachments?.length, attachments: body.attachments?.map((a: any) => ({ type: a.type, path: a.path, base64Length: a.base64?.length })) })
      const authFetcher = createAuthFetcher(bearer)
      const { stream } = createAgent(buildAgentParams(body, bearer!), authFetcher)
      for await (const action of stream({
        query: body.query,
        messages: body.messages,
        skills: body.skills,
        attachments: body.attachments,
      })) {
        yield sse(JSON.stringify(action))
      }
    } catch (error) {
      console.error(error)
      const message = error instanceof Error && error.message.trim()
        ? error.message
        : 'Internal server error'
      yield sse(JSON.stringify({
        type: 'error',
        message,
      }))
    }
    // Send a final done marker so the Go SSE scanner sees a clean termination
    // before the HTTP chunked stream closes.
    yield sse('[DONE]')
  }, {
    body: AgentModel.extend({
      query: z.string(),
    }),
  })
  .post('/trigger-schedule', async ({ body, bearer }) => {
    const authFetcher = createAuthFetcher(bearer)
    const { triggerSchedule } = createAgent(buildAgentParams(body, bearer!), authFetcher)
    return triggerSchedule({
      schedule: body.schedule,
      messages: body.messages,
      skills: body.skills,
    })
  }, {
    body: AgentModel.extend({
      schedule: ScheduleModel,
    }),
  })
  .post('/summarize', async ({ body }) => {
    const mc = body.model as ModelConfig
    const model = createModel(mc)
    // Build messages with system message at the beginning (required by Jinja template processing)
    const systemMessage = {
      role: 'system' as const,
      content: [
        'You are a precise conversation summarizer.',
        'Produce a concise summary of the conversation below.',
        'Preserve: key facts, user preferences, decisions made, action items, and important context.',
        'Omit: greetings, filler, tool call details, and redundant exchanges.',
        'Output ONLY the summary text, no preamble.',
      ].join('\n'),
    }
    const messages = SYSTEM_SAFE_PROVIDERS.has(mc.clientType)
      ? [systemMessage, ...body.messages]
      : [systemMessage, ...body.messages.map((m: any) => m.role === 'system' ? { ...m, role: 'user' } : m)]
    const { text, usage } = await generateText({
      model,
      messages,
    })
    return {
      summary: text,
      usage: {
        prompt_tokens: usage.promptTokens,
        completion_tokens: usage.completionTokens,
        total_tokens: (usage.promptTokens ?? 0) + (usage.completionTokens ?? 0),
      },
    }
  }, {
    body: z.object({
      model: ModelConfigModel,
      messages: z.array(z.any()),
    }),
  })
