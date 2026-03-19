import { AgentAuthContext, IdentityContext } from '../types'

export const buildIdentityHeaders = (identity: IdentityContext, auth: AgentAuthContext, reqId?: string) => {
  // auth.bearer already includes "Bearer " prefix from server
  const headers: Record<string, string> = {
    Authorization: auth.bearer.startsWith('Bearer ') ? auth.bearer : `Bearer ${auth.bearer}`,
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