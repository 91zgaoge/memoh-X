import {
  generateText,
  ImagePart,
  LanguageModelUsage,
  ModelMessage,
  stepCountIs,
  streamText,
  ToolSet,
  UserModelMessage,
} from 'ai'

import {
  AgentInput,
  AgentParams,
  AgentSkill,
  allActions,
  Schedule,
  SYSTEM_SAFE_PROVIDERS,
} from './types'
import { system, schedule, heartbeat, user, subagentSystem } from './prompts'
import { AuthFetcher } from './index'
import { createModel, getProviderOptions } from './model'
import { AgentAction } from './types/action'
import { SubagentRegistry } from './registry'
import {
  extractAttachmentsFromText,
  stripAttachmentsFromMessages,
  dedupeAttachments,
  AttachmentsStreamExtractor,
} from './utils/attachments'
import type { ContainerFileAttachment } from './types/attachment'
import { withRetry, isRetryableLLMError } from './utils/retry'
// // import { getMCPTools } from './tools/mcp'  // Disabled for troubleshooting  // Disabled for troubleshooting
import { getTools } from './tools'
import { wrapToolsWithLoopDetection, clearLoopDetectionState } from './tools/loop-detection'
import { buildIdentityHeaders } from './utils/headers'
import { createTierTools } from './tools/tier'
import { normalizeBaseUrl } from './utils/url'
import {
  truncateToolResult,
  sanitizeToolChunkMetadata,
  truncateMessagesForTransport,
  stripReasoningFromMessages,
} from './utils/sse'
import type { SystemMode } from './prompts/system'

import {
  HEAD_RATIO,
  TAIL_RATIO,
  CHAR_BUDGETS,
  FILE_SIZE_WARN_THRESHOLD,
  SYSTEM_FILE_CACHE_TTL_MS,
} from './config'

function truncateHeadTail(content: string, maxChars: number): string {
  if (maxChars <= 0 || !content) return ''
  if (content.length <= maxChars) return content
  const headChars = Math.floor(maxChars * HEAD_RATIO)
  const tailChars = Math.floor(maxChars * TAIL_RATIO)
  const head = content.slice(0, headChars)
  const tail = content.slice(-tailChars)
  return `${head}\n\n[...truncated — read the full file with the \`read\` tool for complete content...]\n\n${tail}`
}

export const createAgent = (
  {
    model: modelConfig,
    backgroundModel: backgroundModelConfig,
    activeContextTime = 24 * 60,
    language = 'Same as the user input',
    timezone = 'UTC',
    allowedActions = allActions,
    channels = [],
    skills = [],
    mcpConnections = [],
    currentChannel = 'Unknown Channel',
    identity = {
      botId: '',
      containerId: '',
      channelIdentityId: '',
      displayName: '',
    },
    auth,
    botIdentity = '',
    botSoul = '',
    botTask = '',
    allowSelfEvolution = true,
    botTeam = '',
    teamMembers = [] as string[],
    callDepth = 0,
  }: AgentParams,
  fetch: AuthFetcher,
) => {
  const model = createModel(modelConfig)
  const providerOptions = getProviderOptions(modelConfig)
  const backgroundModel = backgroundModelConfig ? createModel(backgroundModelConfig) : null
  const registry = new SubagentRegistry()
  const enabledSkills: AgentSkill[] = []

  const enableSkill = async (skill: string): Promise<{ content: string; description: string } | null> => {
    const agentSkill = skills.find((s) => s.name === skill)
    if (!agentSkill) return null
    let content = agentSkill.content
    if (!content) {
      content = await readSkillContent(skill)
    }
    if (content && !enabledSkills.some((s) => s.name === skill)) {
      enabledSkills.push({ ...agentSkill, content })
    }
    return content ? { content, description: agentSkill.description } : null
  }

  const getEnabledSkills = () => {
    return enabledSkills.map((skill) => skill.name)
  }

  const mcpHeaders = (): Record<string, string> => {
    const h: Record<string, string> = {
      'Content-Type': 'application/json',
      Accept: 'application/json, text/event-stream',
      Authorization: `Bearer ${auth.bearer}`,
    }
    if (identity.channelIdentityId) {
      h['X-Memoh-Channel-Identity-Id'] = identity.channelIdentityId
    }
    return h
  }

  const mcpToolsURL = `${normalizeBaseUrl(auth.baseUrl)}/bots/${identity.botId}/tools`

  const readContainerFile = async (path: string): Promise<string> => {
    if (!auth?.bearer || !identity.botId) return ''
    const body = JSON.stringify({
      jsonrpc: '2.0',
      id: `read-${path}`,
      method: 'tools/call',
      params: { name: 'read', arguments: { path } },
    })
    const response = await fetch(mcpToolsURL, { method: 'POST', headers: mcpHeaders(), body })
    if (!response.ok) return ''
    const data = await response.json().catch(() => ({}))
    const structured =
      data?.result?.structuredContent ?? data?.result?.content?.[0]?.text
    if (typeof structured === 'string') {
      try {
        const parsed = JSON.parse(structured)
        return typeof parsed?.content === 'string' ? parsed.content : ''
      } catch {
        return structured
      }
    }
    if (typeof structured === 'object' && structured?.content) {
      return typeof structured.content === 'string' ? structured.content : ''
    }
    return ''
  }

  const skillContentCache = new Map<string, Promise<string>>()

  const readSkillContent = (skillName: string): Promise<string> => {
    if (!skillContentCache.has(skillName)) {
      const promise = readContainerFile(`/data/.skills/${skillName}/SKILL.md`)
        .then((content) => {
          if (!content) {
            skillContentCache.delete(skillName)
            console.warn(`[${identity.botId}] failed to load skill content for "${skillName}" from container`)
          }
          return content
        })
      skillContentCache.set(skillName, promise)
    }
    return skillContentCache.get(skillName)!
  }

  const TOOL_CATEGORIES: Array<{ label: string; tools: string[]; desc: string }> = [
    { label: 'File', tools: ['read', 'write', 'list', 'edit'], desc: '/data/ (private), /shared/ (cross-bot)' },
    { label: 'Shell', tools: ['exec'], desc: 'run commands in container' },
    { label: 'Web', tools: ['web_search', 'web_fetch'], desc: 'search & fetch web content' },
    { label: 'Memory', tools: ['search_memory', 'query_history'], desc: 'search memories & conversation history' },
    { label: 'Knowledge', tools: ['knowledge_read', 'knowledge_write'], desc: 'read & write bot knowledge base' },
    { label: 'Message', tools: ['send', 'react', 'lookup_channel_user'], desc: 'send messages, reactions & user lookup' },
    { label: 'Image', tools: ['generate_image'], desc: 'generate image from text prompt (async, auto-delivered)' },
    { label: 'Schedule', tools: ['create_schedule', 'list_schedule', 'get_schedule', 'update_schedule', 'delete_schedule'], desc: 'manage cron-based recurring tasks' },
    { label: 'Skills', tools: ['use_skill', 'discover_skills', 'fork_skill'], desc: 'activate, search & import skills' },
    { label: 'Team', tools: ['call_agent'], desc: 'delegate tasks to team member bots' },
    { label: 'Subagent', tools: ['list_subagents', 'create_subagent', 'delete_subagent', 'query_subagent', 'spawn_subagent', 'check_subagent_run', 'kill_subagent_run', 'steer_subagent', 'list_subagent_runs'], desc: 'create & manage sub-agents. ONLY use spawn_subagent when 2+ independent long-running tasks need parallel execution. For simple questions, single-step tasks, or sequential work — do it yourself, never spawn.' },
    { label: 'OpenViking', tools: ['ov_initialize', 'ov_find', 'ov_search', 'ov_read', 'ov_abstract', 'ov_overview', 'ov_ls', 'ov_tree', 'ov_add_resource', 'ov_rm', 'ov_session_commit'], desc: 'context database (see TOOLS.md for details)' },
    { label: 'Admin', tools: ['admin_list_bots', 'admin_create_bot', 'admin_delete_bot', 'admin_list_models', 'admin_create_model', 'admin_delete_model', 'admin_list_providers', 'admin_create_provider', 'admin_update_provider'], desc: 'manage bots, models & providers' },
  ]

  const generateToolContext = (toolNames: string[] = []): string => {
    const sections: string[] = []
    const externalMcps = mcpConnections.filter(c => c.name !== 'builtin')
    if (externalMcps.length > 0) {
      sections.push(
        '### External Tools (MCP)\n' +
        externalMcps.map(c => `- **${c.name}** (${c.type})`).join('\n'),
      )
    }

    const has = (name: string) => toolNames.includes(name)
    const lines: string[] = []
    const categorized = new Set<string>()

    for (const cat of TOOL_CATEGORIES) {
      const present = cat.tools.filter(has)
      if (present.length === 0) continue
      present.forEach(t => categorized.add(t))
      lines.push(`- ${cat.label}: ${present.map(t => `\`${t}\``).join(', ')} — ${cat.desc}`)
    }

    const uncategorized = toolNames.filter(n => !categorized.has(n))
    for (const name of uncategorized) {
      lines.push(`- \`${name}\``)
    }

    if (lines.length > 0) {
      sections.push(
        '### Available Tools\n' +
        lines.join('\n') +
        '\n\nCLI tools (use via `exec`): `agent-browser`, `clawhub`, `actionbook`' +
        '\n\nFor detailed documentation, read /data/TOOLS.md',
      )
    }

    return sections.join('\n\n')
  }

  const systemFileCache: {
    key: string
    data: { identityContent: string; soulContent: string; toolsContent: string }
    expiry: number
  } | null = { key: '', data: { identityContent: '', soulContent: '', toolsContent: '' }, expiry: 0 }

  const loadSystemFiles = async () => {
    if (!auth?.bearer || !identity.botId) {
      return {
        identityContent: botIdentity,
        soulContent: botSoul,
        toolsContent: '',
      }
    }

    const cacheKey = `${identity.botId}:${botIdentity}:${botSoul}`
    if (systemFileCache && systemFileCache.key === cacheKey && Date.now() < systemFileCache.expiry) {
      return systemFileCache.data
    }

    const readViaMCP = readContainerFile

    // Async restore: write DB persona content back to container if the file is
    // missing/empty. Fires in the background; failures are silently ignored so
    // they never block the current request.
    const restoreViaMCP = (path: string, content: string): void => {
      if (!content.trim()) return
      const body = JSON.stringify({
        jsonrpc: '2.0',
        id: `write-${path}`,
        method: 'tools/call',
        params: { name: 'write', arguments: { path, content } },
      })
      fetch(mcpToolsURL, { method: 'POST', headers: mcpHeaders(), body }).catch((e) => {
        console.warn(`[restore] failed to write ${path}:`, e?.message ?? e)
      })
    }

    const needIdentity = !botIdentity
    const needSoul = !botSoul
    const needTools = true

    const mcpReads: Promise<string>[] = [
      needIdentity ? readViaMCP('IDENTITY.md') : Promise.resolve(''),
      needSoul ? readViaMCP('SOUL.md') : Promise.resolve(''),
      needTools ? readViaMCP('TOOLS.md') : Promise.resolve(''),
    ]
    const [mcpIdentity, mcpSoul, toolsContent] = await Promise.all(mcpReads)

    // Self-healing: if DB has persona content but container file is empty,
    // asynchronously restore the file so evolution can read it next time.
    if (botIdentity && !mcpIdentity.trim()) restoreViaMCP('IDENTITY.md', botIdentity)
    if (botSoul && !mcpSoul.trim()) restoreViaMCP('SOUL.md', botSoul)

    const result = {
      identityContent: botIdentity || mcpIdentity,
      soulContent: botSoul || mcpSoul,
      toolsContent,
    }

    if (result.soulContent.length > FILE_SIZE_WARN_THRESHOLD) {
      console.warn(`[${identity.botId}] SOUL.md is ${result.soulContent.length} chars — consider distilling to reduce token consumption`)
    }
    if (result.toolsContent.length > FILE_SIZE_WARN_THRESHOLD) {
      console.warn(`[${identity.botId}] TOOLS.md is ${result.toolsContent.length} chars — consider distilling to reduce token consumption`)
    }

    Object.assign(systemFileCache!, { key: cacheKey, data: result, expiry: Date.now() + SYSTEM_FILE_CACHE_TTL_MS })

    return result
  }

  const generateSystemPrompt = async (mode: SystemMode = 'full', toolNames: string[] = []) => {
    const { identityContent, soulContent, toolsContent } =
      await loadSystemFiles()
    const budget = CHAR_BUDGETS[mode]
    return system({
      date: new Date(),
      language,
      timezone,
      maxContextLoadTime: activeContextTime,
      channels,
      currentChannel,
      skills,
      enabledSkills,
      identityContent,
      soulContent: truncateHeadTail(soulContent, budget.soul),
      toolsContent: truncateHeadTail(toolsContent, budget.tools),
      toolContext: generateToolContext(toolNames),
      taskContent: botTask,
      allowSelfEvolution,
      teamContent: botTeam || undefined,
      mode,
    })
  }

  // MCP disabled for troubleshooting
  // Create tier tools once per agent instance so enabled-tools state is
  // scoped to this session and persists across turns (but not across agents).
  const tier = createTierTools({ auth, identity, fetch })

  const getAgentTools = async (sessionId?: string) => {
    const tools = getTools(allowedActions, { fetch, model: modelConfig, backgroundModel: backgroundModelConfig, identity, auth, enableSkill, mcpConnections, registry, teamMembers, callDepth })
    const { list_available_tools, enable_tools } = tier
    const merged = { list_available_tools, enable_tools, ...tools } as ToolSet
    const toolNames = Object.keys(merged)
    const wrappedTools = sessionId ? wrapToolsWithLoopDetection(merged, sessionId) : merged
    return {
      tools: wrappedTools,
      toolNames,
      close: async () => {
        if (sessionId) clearLoopDetectionState(sessionId)
      },
    }
  }

  const generateUserPrompt = (input: AgentInput) => {
    console.log('[Agent generateUserPrompt] Input:', { query: input.query?.slice(0, 200), attachmentCount: input.attachments?.length, attachments: input.attachments?.map((a: any) => ({ type: a.type, path: (a as any).path, base64Length: (a as any).base64?.length })) })
    const images = input.attachments.filter(
      (attachment) => attachment.type === 'image',
    )
    const files = input.attachments.filter(
      (a): a is ContainerFileAttachment => a.type === 'file',
    )
    const text = user(input.query, {
      channelIdentityId: identity.channelIdentityId || identity.contactId || '',
      displayName: identity.displayName || identity.contactName || 'User',
      channel: currentChannel,
      conversationType: identity.conversationType || 'direct',
      date: new Date(),
      attachments: files,
    })
    const userMessage: UserModelMessage = {
      role: 'user',
      content: [
        { type: 'text', text },
        ...images.map(
          (image) => ({ type: 'image', image: image.base64 }) as ImagePart,
        ),
      ],
    }
    return userMessage
  }

  const sanitizeMessages = (messages: ModelMessage[]): ModelMessage[] => {
    const supportedRoles = new Set(['user', 'assistant', 'system', 'tool'])
    const supportedTypes = new Set(['text', 'image', 'file', 'tool-call', 'tool-result', 'reasoning'])

    // First filter valid messages
    const filtered = messages.filter((msg) => {
      // Drop messages with unsupported roles (e.g. item_reference from Responses API).
      if (!msg || typeof msg !== 'object') return false
      const role = (msg as Record<string, unknown>).role
      if (typeof role !== 'string' || !supportedRoles.has(role)) return false
      // Drop messages that have a non-standard "type" field at the top level.
      const msgType = (msg as Record<string, unknown>).type
      if (typeof msgType === 'string' && msgType !== '' && !supportedTypes.has(msgType)) return false
      return true
    })

    // Separate system messages from other messages
    const systemMsgs: ModelMessage[] = []
    const otherMsgs: ModelMessage[] = []

    for (const msg of filtered) {
      const role = (msg as Record<string, unknown>).role as string
      // Transform message content
      let transformedMsg = msg
      if (role === 'system' && !SYSTEM_SAFE_PROVIDERS.has(modelConfig.clientType)) {
        transformedMsg = { ...msg, role: 'user' } as ModelMessage
      }
      if (Array.isArray(msg.content)) {
        const original = msg.content as Array<Record<string, unknown>>
        const filteredContent = original.filter((part) => {
          if (!part || typeof part !== 'object') return true
          const t = part.type
          if (!t || typeof t !== 'string') return true
          return supportedTypes.has(t)
        })
        if (filteredContent.length !== original.length) {
          if (filteredContent.length === 0) {
            transformedMsg = { ...msg, content: [{ type: 'text', text: '' }] } as ModelMessage
          } else {
            transformedMsg = { ...msg, content: filteredContent } as ModelMessage
          }
        }
      }

      // Separate system messages (must be at beginning for Jinja template processing)
      if (role === 'system') {
        systemMsgs.push(transformedMsg)
      } else {
        otherMsgs.push(transformedMsg)
      }
    }

    // System messages must be at the beginning (required by Jinja template processing)
    return [...systemMsgs, ...otherMsgs]
  }

  // Normalize AI SDK v6 usage fields to the legacy names expected by the
  // Go backend (gatewayUsage) and the web frontend (promptTokens, etc.).
  const normalizeUsage = (usage: LanguageModelUsage | null) => {
    if (!usage) return { promptTokens: 0, completionTokens: 0, totalTokens: 0 }
    const input = (usage as Record<string, unknown>).inputTokens as number | undefined
    const output = (usage as Record<string, unknown>).outputTokens as number | undefined
    const prompt = (usage as Record<string, unknown>).promptTokens as number | undefined
    const completion = (usage as Record<string, unknown>).completionTokens as number | undefined
    const p = prompt ?? input ?? 0
    const c = completion ?? output ?? 0
    return {
      promptTokens: p,
      completionTokens: c,
      totalTokens: usage.totalTokens ?? (p + c),
    }
  }

  // Detect file types from attachments and auto-enable relevant skills
  const autoEnableSkillsFromAttachments = async (attachments: AgentAttachment[]) => {
    const files = attachments.filter((a): a is ContainerFileAttachment => a.type === 'file')
    for (const file of files) {
      const lowerPath = file.path?.toLowerCase() || ''
      let skillName: string | null = null
      if (lowerPath.endsWith('.xls') || lowerPath.endsWith('.xlsx') || lowerPath.endsWith('.xlsm')) {
        skillName = 'xlsx'
      } else if (lowerPath.endsWith('.docx') || lowerPath.endsWith('.doc')) {
        skillName = 'docx'
      } else if (lowerPath.endsWith('.pptx') || lowerPath.endsWith('.ppt')) {
        skillName = 'pptx'
      } else if (lowerPath.endsWith('.pdf')) {
        skillName = 'pdf'
      }

      if (skillName && !enabledSkills.some((s) => s.name === skillName)) {
        // Try to find skill in available skills list first
        const agentSkill = skills.find((s) => s.name === skillName)
        if (agentSkill) {
          // Use pre-loaded skill
          let content = agentSkill.content
          if (!content) {
            content = await readSkillContent(skillName)
          }
          if (content) {
            enabledSkills.push({ ...agentSkill, content })
            console.log(`[${identity.botId}] Auto-enabled skill: ${skillName}`)
          }
        } else {
          // Skill not in available list - try to load directly from container
          const content = await readSkillContent(skillName)
          if (content) {
            enabledSkills.push({
              name: skillName,
              description: `Auto-loaded ${skillName} skill for file processing`,
              content,
            })
            console.log(`[${identity.botId}] Auto-loaded skill from container: ${skillName} (content length: ${content.length})`)
          } else {
            console.warn(`[${identity.botId}] Failed to load skill content for: ${skillName}`)
          }
        }
      }
    }
  }

  const ask = async (input: AgentInput) => {
    const userPrompt = generateUserPrompt(input)
    await Promise.all(input.skills.map((skill) => enableSkill(skill)))
    // Auto-enable skills based on file attachments
    await autoEnableSkillsFromAttachments(input.attachments)
    const sessionId = `ask:${identity.botId}:${Date.now()}`
    const { tools, toolNames, close } = await getAgentTools(sessionId)
    console.log(`[${identity.botId}] Enabled skills before generating system prompt:`, enabledSkills.map(s => ({ name: s.name, contentLength: s.content?.length || 0 })))
    const systemPrompt = await generateSystemPrompt('full', toolNames)
    console.log(`[${identity.botId}] System prompt length: ${systemPrompt.length}, includes xlsx: ${systemPrompt.includes('xlsx')}`)
    // Build messages with system message at the beginning (required by Jinja template processing)
    // SystemModelMessage requires content to be a string, not an array
    // Remove any existing system messages from input to avoid duplicates
    const nonSystemMessages = sanitizeMessages(input.messages).filter((msg) => (msg as Record<string, unknown>).role !== 'system')
    const systemMessage = {
      role: 'system' as const,
      content: systemPrompt,
    }
    const messages = [systemMessage, ...nonSystemMessages, userPrompt]
    const { response, reasoning, text, usage } = await withRetry(
      () =>
        generateText({
          model,
          messages,
          stopWhen: stepCountIs(Infinity),
          onFinish: async () => {
            await close()
          },
          tools,
          providerOptions,
        }),
      isRetryableLLMError,
    )
    const { cleanedText, attachments: textAttachments } =
      extractAttachmentsFromText(text)
    const { messages: strippedMessages, attachments: messageAttachments } =
      stripAttachmentsFromMessages(response.messages)
    const cleanedMessages = stripReasoningFromMessages(
      truncateMessagesForTransport(strippedMessages),
    )
    const allAttachments = dedupeAttachments([
      ...textAttachments,
      ...messageAttachments,
    ])
    return {
      messages: cleanedMessages,
      reasoning: reasoning.map((part) => part.text),
      usage: normalizeUsage(usage),
      text: cleanedText,
      attachments: allAttachments,
      skills: getEnabledSkills(),
    }
  }

  const askAsSubagent = async (params: {
    input: string;
    name: string;
    description: string;
    messages: ModelMessage[];
    abortSignal?: AbortSignal;
  }) => {
    const userPrompt: UserModelMessage = {
      role: 'user',
      content: [{ type: 'text', text: params.input }],
    }
    const rolePath = `/data/subagent-roles/${params.name}`
    const [roleIdentity, roleSoul, roleTask] = await Promise.all([
      readContainerFile(`${rolePath}/identity.md`).catch(() => ''),
      readContainerFile(`${rolePath}/soul.md`).catch(() => ''),
      readContainerFile(`${rolePath}/task.md`).catch(() => ''),
    ])
    const sessionId = `subagent:${identity.botId}:${params.name}:${Date.now()}`
    const { tools, toolNames, close } = await getAgentTools(sessionId)
    const toolContext = generateToolContext(toolNames)
    const generateSubagentSystemPrompt = () => {
      return subagentSystem({
        date: new Date(),
        name: params.name,
        description: params.description,
        timezone,
        toolContext,
        skills,
        identityContent: roleIdentity,
        soulContent: roleSoul,
        taskContent: roleTask,
      })
    }
    const subagentModel = backgroundModel ?? model
    const subagentSystemPrompt = generateSubagentSystemPrompt()
    // Build messages with system message at the beginning (required by Jinja template processing)
    // SystemModelMessage requires content to be a string, not an array
    // Remove any existing system messages from input to avoid duplicates
    const nonSystemMessages = sanitizeMessages(params.messages).filter((msg) => (msg as Record<string, unknown>).role !== 'system')
    const subagentSystemMessage = {
      role: 'system' as const,
      content: subagentSystemPrompt,
    }
    const messagesWithSystem = [subagentSystemMessage, ...nonSystemMessages, userPrompt]
    const { response, reasoning, text, usage } = await withRetry(
      () =>
        generateText({
          model: subagentModel,
          messages: messagesWithSystem,
          stopWhen: stepCountIs(Infinity),
          onFinish: async () => {
            await close()
          },
          tools,
          abortSignal: params.abortSignal,
          providerOptions,
        }),
      isRetryableLLMError,
    )
    return {
      messages: stripReasoningFromMessages(
        truncateMessagesForTransport([userPrompt, ...response.messages]),
      ),
      reasoning: reasoning.map((part) => part.text),
      usage: normalizeUsage(usage),
      text,
      skills: getEnabledSkills(),
    }
  }

  const streamAsSubagent = async (params: {
    input: string;
    name: string;
    description: string;
    messages: ModelMessage[];
    abortSignal?: AbortSignal;
    onDelta?: (delta: string) => void;
    onAttachment?: (attachment: { type: 'file'; path: string }) => void;
  }) => {
    const userPrompt: UserModelMessage = {
      role: 'user',
      content: [{ type: 'text', text: params.input }],
    }
    const rolePath = `/data/subagent-roles/${params.name}`
    const [roleIdentity, roleSoul, roleTask] = await Promise.all([
      readContainerFile(`${rolePath}/identity.md`).catch(() => ''),
      readContainerFile(`${rolePath}/soul.md`).catch(() => ''),
      readContainerFile(`${rolePath}/task.md`).catch(() => ''),
    ])
    const sessionId = `subagent:${identity.botId}:${params.name}:${Date.now()}`
    const { tools, toolNames: _subToolNames, close } = await getAgentTools(sessionId)
    const toolContext = generateToolContext(_subToolNames)
    const subagentModel = backgroundModel ?? model
    const sysPrompt = subagentSystem({
      date: new Date(),
      name: params.name,
      description: params.description,
      timezone,
      toolContext,
      skills,
      identityContent: roleIdentity,
      soulContent: roleSoul,
      taskContent: roleTask,
    })
    // Build messages with system message at the beginning (required by Jinja template processing)
    // SystemModelMessage requires content to be a string, not an array
    // Remove any existing system messages from input to avoid duplicates
    const nonSystemMessages = sanitizeMessages(params.messages).filter((msg) => (msg as Record<string, unknown>).role !== 'system')
    const systemMessage = {
      role: 'system' as const,
      content: sysPrompt,
    }
    const messages = [systemMessage, ...nonSystemMessages, userPrompt]

    const result: { messages: ModelMessage[]; reasoning: string[]; usage: LanguageModelUsage | null } = {
      messages: [], reasoning: [], usage: null,
    }
    let closeCalled = false
    const safeClose = async () => { if (!closeCalled) { closeCalled = true; await close() } }

    const { fullStream } = streamText({
      model: subagentModel,
      messages,
      stopWhen: stepCountIs(Infinity),
      tools,
      abortSignal: params.abortSignal,
      providerOptions,
      onFinish: async ({ usage, reasoning, response }) => {
        await safeClose()
        result.usage = usage as never
        result.reasoning = reasoning.map((part) => part.text)
        result.messages = response.messages
      },
    })

    try {
      for await (const chunk of fullStream) {
        if (chunk.type === 'text-delta' && params.onDelta) {
          params.onDelta(chunk.text)
        }
        if (chunk.type === 'tool-result' && params.onAttachment && FILE_WRITE_TOOLS.has(chunk.toolName) && isWriteSuccess(chunk.output)) {
          const writePath = extractWritePath(chunk.input)
          if (writePath && isDeliverableWrite(writePath)) {
            params.onAttachment({ type: 'file', path: writePath })
          }
        }
      }
    } finally {
      await safeClose()
    }

    return {
      messages: stripReasoningFromMessages(
        truncateMessagesForTransport([userPrompt, ...result.messages]),
      ),
      reasoning: result.reasoning,
      usage: normalizeUsage(result.usage),
      skills: getEnabledSkills(),
    }
  }

  const triggerSchedule = async (params: {
    schedule: Schedule;
    messages: ModelMessage[];
    skills: string[];
  }) => {
    const isHeartbeat = params.schedule.triggerType === 'heartbeat'
    const promptText = isHeartbeat
      ? heartbeat({ schedule: params.schedule, date: new Date(), timezone })
      : schedule({ schedule: params.schedule, date: new Date(), timezone })
    const scheduleMessage: UserModelMessage = {
      role: 'user',
      content: [
        {
          type: 'text',
          text: promptText,
        },
      ],
    }
    const messages = [...sanitizeMessages(params.messages), scheduleMessage]
    await Promise.all(params.skills.map((skill) => enableSkill(skill)))
    const sessionId = `schedule:${identity.botId}:${params.schedule.id}:${Date.now()}`
    const { tools, toolNames: schedToolNames, close } = await getAgentTools(sessionId)
    const systemPromptText = await generateSystemPrompt(isHeartbeat ? 'micro' : 'minimal', schedToolNames)
    const scheduleModel = backgroundModel ?? model
    // Build messages with system message at the beginning (required by Jinja template processing)
    // SystemModelMessage requires content to be a string, not an array
    // Remove any existing system messages from input to avoid duplicates
    const nonSystemMessages = messages.filter((msg) => (msg as Record<string, unknown>).role !== 'system')
    const scheduleSystemMessage = {
      role: 'system' as const,
      content: systemPromptText,
    }
    const messagesWithSystem = [scheduleSystemMessage, ...nonSystemMessages]
    const { response, reasoning, text, usage } = await withRetry(
      () =>
        generateText({
          model: scheduleModel,
          messages: messagesWithSystem,
          stopWhen: stepCountIs(Infinity),
          onFinish: async () => {
            await close()
          },
          tools,
          providerOptions,
        }),
      isRetryableLLMError,
    )
    return {
      messages: stripReasoningFromMessages(
        truncateMessagesForTransport([scheduleMessage, ...response.messages]),
      ),
      reasoning: reasoning.map((part) => part.text),
      usage: normalizeUsage(usage),
      text,
      skills: getEnabledSkills(),
    }
  }

  const resolveStreamErrorMessage = (raw: unknown): string => {
    if (raw instanceof Error && raw.message.trim()) {
      return raw.message
    }
    if (typeof raw === 'string' && raw.trim()) {
      return raw
    }
    if (raw && typeof raw === 'object') {
      const candidate = raw as { message?: unknown; error?: unknown }
      if (typeof candidate.message === 'string' && candidate.message.trim()) {
        return candidate.message
      }
      if (typeof candidate.error === 'string' && candidate.error.trim()) {
        return candidate.error
      }
      if (candidate.error instanceof Error && candidate.error.message.trim()) {
        return candidate.error.message
      }
    }
    return 'Model stream failed'
  }

  // -- Write-tool attachment helpers --
  const FILE_WRITE_TOOLS = new Set(['write', 'save_file', 'create_file', 'write_file'])
  const isDeliverableWrite = (p: string): boolean => {
    if (!p.startsWith('/shared/')) return false
    const dot = p.lastIndexOf('.')
    return dot !== -1 && dot > p.lastIndexOf('/')
  }

  const extractWritePath = (input: unknown): string | null => {
    if (!input || typeof input !== 'object') return null
    const p = (input as Record<string, unknown>).path
    return typeof p === 'string' && p.length > 0 ? p : null
  }

  const isWriteSuccess = (output: unknown): boolean => {
    if (output === undefined || output === null) return false
    if (typeof output === 'string') return !output.toLowerCase().includes('error')
    if (typeof output === 'object') {
      const o = output as Record<string, unknown>
      if (o.isError === true) return false
      const content = o.content
      if (Array.isArray(content)) {
        return !content.some((c: any) => c?.type === 'text' && typeof c.text === 'string' && c.text.toLowerCase().includes('error'))
      }
    }
    return true
  }

  async function* stream(input: AgentInput): AsyncGenerator<AgentAction> {
    const userPrompt = generateUserPrompt(input)
    await Promise.all(input.skills.map((skill) => enableSkill(skill)))
    await autoEnableSkillsFromAttachments(input.attachments)
    const sessionId = `stream:${identity.botId}:${Date.now()}`
    const { tools, toolNames: streamToolNames, close } = await getAgentTools(sessionId)
    const systemPrompt = await generateSystemPrompt('full', streamToolNames)
    const nonSystemMessages = sanitizeMessages(input.messages).filter((msg) => (msg as Record<string, unknown>).role !== 'system')
    const messages = [
      { role: 'system', content: systemPrompt },
      ...nonSystemMessages,
      userPrompt
    ]
    console.log('[Agent stream] Built messages:', messages.length)

    yield { type: 'agent_start', input }

    try {
      // Direct fetch to LLM API (bypass ai SDK due to Bun compatibility issues)
      console.log('[Agent stream] Starting fetch to', modelConfig.baseUrl)
      const fetchUrl = `${modelConfig.baseUrl.replace(/\/$/, '')}/chat/completions`
      console.log('[Agent stream] Fetch URL:', fetchUrl)
      const response = await fetch(fetchUrl, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${modelConfig.apiKey}`,
        },
        body: JSON.stringify({
          model: modelConfig.modelId,
          messages: messages.map((m: any) => ({
            role: m.role,
            content: typeof m.content === 'string' ? m.content : JSON.stringify(m.content),
          })),
          stream: false,
        }),
      })

      if (!response.ok) {
        const errorText = await response.text()
        console.error(`[Agent stream] LLM API error ${response.status}:`, errorText)
        throw new Error(`LLM API error: ${response.status} ${errorText}`)
      }

      const data = await response.json()
      const content = data.choices?.[0]?.message?.content || ''

      // Simulate streaming
      yield { type: 'text_start' }
      const chunkSize = 50
      for (let i = 0; i < content.length; i += chunkSize) {
        yield { type: 'text_delta', delta: content.slice(i, i + chunkSize) }
      }
      yield { type: 'text_end', metadata: {} }

      await close()

      // Build final messages with assistant response
      const assistantMessage: ModelMessage = {
        role: 'assistant',
        content,
      }
      const finalMessages = [...messages, assistantMessage] as ModelMessage[]

      const { messages: strippedMessages } = stripAttachmentsFromMessages(finalMessages)
      const cleanedMessages = stripReasoningFromMessages(
        truncateMessagesForTransport(strippedMessages),
      ) as ModelMessage[]

      yield {
        type: 'agent_end',
        messages: cleanedMessages,
        reasoning: [],
        usage: { promptTokens: 0, completionTokens: 0, totalTokens: content.length },
        skills: getEnabledSkills(),
      }
    } catch (err) {
      console.error('[Agent stream] Error:', err)
      yield { type: 'text_delta', delta: `Error: ${err instanceof Error ? err.message : String(err)}` }
      yield { type: 'text_end', metadata: {} }

      // Even on error, yield agent_end with current messages
      const { messages: strippedMessages } = stripAttachmentsFromMessages(messages as ModelMessage[])
      const cleanedMessages = stripReasoningFromMessages(
        truncateMessagesForTransport(strippedMessages),
      ) as ModelMessage[]

      yield {
        type: 'agent_end',
        messages: cleanedMessages,
        reasoning: [],
        usage: { promptTokens: 0, completionTokens: 0, totalTokens: 0 },
        skills: getEnabledSkills(),
      }
    }
  }

  return {
    stream,
    ask,
    askAsSubagent,
    streamAsSubagent,
    triggerSchedule,
  }
}
