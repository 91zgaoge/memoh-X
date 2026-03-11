import { ContainerFileAttachment } from '../types'

export interface UserParams {
  channelIdentityId: string
  displayName: string
  channel: string
  conversationType: string
  date: Date
  attachments: ContainerFileAttachment[]
}

function getFileTypeHint(path: string): { hint: string; skill: string } {
  const lower = path.toLowerCase()
  if (lower.endsWith('.xls') || lower.endsWith('.xlsx') || lower.endsWith('.xlsm')) {
    return { hint: ' (Excel spreadsheet)', skill: 'xlsx' }
  }
  if (lower.endsWith('.pdf')) {
    return { hint: ' (PDF document)', skill: 'pdf' }
  }
  if (lower.endsWith('.docx') || lower.endsWith('.doc')) {
    return { hint: ' (Word document)', skill: 'docx' }
  }
  if (lower.endsWith('.pptx') || lower.endsWith('.ppt')) {
    return { hint: ' (PowerPoint presentation)', skill: 'pptx' }
  }
  return { hint: '', skill: '' }
}

export const user = (
  query: string,
  { channelIdentityId, displayName, channel, conversationType, date, attachments }: UserParams
) => {
  const headers: Record<string, any> = {
    'channel-identity-id': channelIdentityId,
    'display-name': displayName,
    'channel': channel,
    'conversation-type': conversationType,
    'time': date.toISOString(),
  }

  // Build file processing instructions based on attachments
  const fileInstructions: string[] = []

  // Include file attachments with type hints
  if (attachments.length > 0) {
    headers['attachments'] = attachments.map(attachment => {
      const { hint, skill } = getFileTypeHint(attachment.path)
      if (skill) {
        const isExcel = skill === 'xlsx'
        const skillNote = isExcel ? ' ⚠️ NEVER use rag-documents for Excel files!' : ''
        fileInstructions.push(
          `CRITICAL: Use ".use_skill skillName: \\"${skill}\\" reason: \\"To analyze ${attachment.path}\\""${skillNote} to process ${attachment.path}. DO NOT try to read this file directly.` +
          (isExcel ? '\n\n⚠️ **IMPORTANT**: This is an Excel file. Your FIRST action MUST be to use the xlsx skill. Do NOT use rag-documents skill.' : '')
        )
      }
      return `${attachment.path}${hint}`
    })
  }

  const fileInstructionText = fileInstructions.length > 0
    ? `\n\n**File Processing Instructions:**\n${fileInstructions.join('\n')}\n`
    : ''

  return `
---
${Bun.YAML.stringify(headers)}
---
${query}${fileInstructionText}
  `.trim()
}