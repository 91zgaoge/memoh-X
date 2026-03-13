import { AgentAuthContext, IdentityContext } from '../types'

export const buildIdentityHeaders = (identity: IdentityContext, auth: AgentAuthContext, reqId?: string) => {
  const headers: Record<string, string> = {
    Authorization: `Bearer ${auth.bearer}`,
  }
  if (identity.channelIdentityId) {
    headers['X-Memoh-Channel-Identity-Id'] = identity.channelIdentityId
  }
  if (identity.sessionToken) {
    headers['X-Memoh-Session-Token'] = identity.sessionToken
  }
  if (identity.currentPlatform) {
    headers['X-Memoh-Current-Platform'] = identity.currentPlatform
  }
  if (identity.replyTarget) {
    headers['X-Memoh-Reply-Target'] = identity.replyTarget
  }
  if (reqId) {
    headers['X-Memoh-Req-ID'] = reqId
  }
  return headers
}