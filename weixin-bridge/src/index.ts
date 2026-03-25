import express, { type Request, type Response } from 'express';
import { WeixinBot, type IncomingMessage } from '@pinixai/weixin-bot';

// Try to import qrcode, fallback to external service if not available
let QRCode: any = null;
try {
  QRCode = require('qrcode');
} catch {
  console.log('[Info] qrcode package not available, using external QR code service');
}

interface PendingMessage {
  task_id: string;
  reply: string;
  sender: string;
  chat_id: string;
}

interface BridgeState {
  status: 'pending' | 'connecting' | 'connected' | 'disconnected';
  qrcodeUrl: string | null;
  qrcodeData: string | null; // Raw data for QR code generation
  lastError: string | null;
  loggedInAt: Date | null;
  scanDetected: boolean;
  confirmationPending: boolean;
  currentUser: string | null; // Current logged in user
  loginHistory: string[]; // History of logged in users
}

// Configuration from environment
const MEMOH_SERVER_URL = process.env.MEMOH_SERVER_URL || 'http://server:8080';
const MEMOH_BOT_ID = process.env.MEMOH_BOT_ID || '';
const MEMOH_API_KEY = process.env.MEMOH_API_KEY || '';
const BRIDGE_PORT = parseInt(process.env.BRIDGE_PORT || '3000');
const TOKEN_PATH = process.env.TOKEN_PATH || '/data/.weixin-bot/credentials.json';
const PUBLIC_BASE_URL = process.env.PUBLIC_BASE_URL || ''; // External access URL, e.g., https://wework.jxtvnet.com/api/weixin-bridge/{bot_id}

// Bridge state
const state: BridgeState = {
  status: 'pending',
  qrcodeUrl: null,
  qrcodeData: null,
  lastError: null,
  loggedInAt: null,
  scanDetected: false,
  confirmationPending: false,
  currentUser: null,
  loginHistory: [],
};

// SSE clients
const sseClients: Set<Response> = new Set();

// Express app for control endpoints
const app = express();
app.use(express.json());

// Helper: broadcast state to all SSE clients
function broadcastState() {
  // Use public base URL for qrcodeUrl if available
  const publicQrUrl = PUBLIC_BASE_URL ? `${PUBLIC_BASE_URL}/qrcode-image` : state.qrcodeUrl;

  const data = JSON.stringify({
    status: state.status,
    qrcodeUrl: publicQrUrl,
    scanDetected: state.scanDetected,
    confirmationPending: state.confirmationPending,
    loggedInAt: state.loggedInAt?.toISOString() || null,
    currentUser: state.currentUser,
    loginHistory: state.loginHistory,
    timestamp: new Date().toISOString(),
  });

  sseClients.forEach((client) => {
    try {
      client.write(`data: ${data}\n\n`);
    } catch (err) {
      // Client disconnected
      sseClients.delete(client);
    }
  });
}

// Health check endpoint
app.get('/health', (_req: Request, res: Response) => {
  res.json({
    status: state.status,
    timestamp: new Date().toISOString(),
  });
});

// QR code image endpoint - generates QR code image directly or redirects to external service
app.get('/qrcode-image', async (_req: Request, res: Response) => {
  const dataToEncode = state.qrcodeData || state.qrcodeUrl;

  if (!dataToEncode) {
    return res.status(404).json({
      success: false,
      error: 'No QR code available',
      status: state.status,
    });
  }

  // If qrcode package is available, use it
  if (QRCode) {
    try {
      const qrBuffer = await QRCode.toBuffer(dataToEncode, {
        type: 'png',
        width: 400,
        margin: 2,
        color: {
          dark: '#000000',
          light: '#ffffff',
        },
      });

      res.setHeader('Content-Type', 'image/png');
      res.setHeader('Cache-Control', 'no-cache, no-store, must-revalidate');
      res.send(qrBuffer);
      return;
    } catch (error) {
      console.error('[QRCode] Failed to generate QR code image:', error);
      // Fall through to external service
    }
  }

  // Fallback: redirect to external QR code service
  const encodedData = encodeURIComponent(dataToEncode);
  const qrServiceUrl = `https://api.qrserver.com/v1/create-qr-code/?size=400x400&data=${encodedData}`;
  res.redirect(qrServiceUrl);
});

// SSE endpoint for real-time status updates
app.get('/events', (req: Request, res: Response) => {
  res.writeHead(200, {
    'Content-Type': 'text/event-stream',
    'Cache-Control': 'no-cache',
    'Connection': 'keep-alive',
  });

  // Send initial state - use public base URL for qrcodeUrl if available
  const publicQrUrl = PUBLIC_BASE_URL ? `${PUBLIC_BASE_URL}/qrcode-image` : state.qrcodeUrl;
  const initialData = JSON.stringify({
    status: state.status,
    qrcodeUrl: publicQrUrl,
    scanDetected: state.scanDetected,
    confirmationPending: state.confirmationPending,
    loggedInAt: state.loggedInAt?.toISOString() || null,
    currentUser: state.currentUser,
    loginHistory: state.loginHistory,
    timestamp: new Date().toISOString(),
  });
  res.write(`data: ${initialData}\n\n`);

  // Add client to set
  sseClients.add(res);

  // Remove client on close
  req.on('close', () => {
    sseClients.delete(res);
  });
});

// QR code endpoint - returns HTML page for browser
app.get('/qrcode', (req: Request, res: Response) => {
  const acceptHeader = req.headers.accept || '';
  const isBrowser = acceptHeader.includes('text/html') || !acceptHeader.includes('application/json');

  if (!state.qrcodeUrl && state.status !== 'connecting') {
    if (isBrowser) {
      res.status(404).send('<h1>二维码未就绪</h1><p>请稍后再试，或查看 <a href="/status">状态</a></p>');
    } else {
      res.status(404).json({
        success: false,
        error: 'No QR code available',
        status: state.status,
      });
    }
    return;
  }

  if (isBrowser) {
    const html = `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>微信扫码登录</title>
  <style>
    * { box-sizing: border-box; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
      margin: 0;
      background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
      padding: 20px;
    }
    .container {
      background: white;
      padding: 40px;
      border-radius: 16px;
      box-shadow: 0 20px 60px rgba(0,0,0,0.3);
      text-align: center;
      max-width: 500px;
      width: 100%;
    }
    h1 { color: #333; margin-bottom: 10px; font-size: 24px; }
    .subtitle { color: #666; margin-bottom: 30px; font-size: 14px; }
    .qr-container {
      background: #f8f9fa;
      padding: 20px;
      border-radius: 12px;
      margin: 20px 0;
      display: flex;
      justify-content: center;
      align-items: center;
      min-height: 280px;
    }
    .qr-image {
      max-width: 100%;
      height: auto;
      border-radius: 8px;
    }
    .qr-placeholder {
      color: #999;
      font-size: 14px;
    }
    .qrcode-url {
      word-break: break-all;
      background: #f0f0f0;
      padding: 12px;
      border-radius: 8px;
      font-family: monospace;
      font-size: 12px;
      margin: 15px 0;
      color: #666;
      cursor: pointer;
      transition: background 0.2s;
    }
    .qrcode-url:hover { background: #e0e0e0; }
    .status {
      margin-top: 20px;
      padding: 12px 24px;
      border-radius: 24px;
      font-size: 14px;
      font-weight: 500;
      display: inline-flex;
      align-items: center;
      gap: 8px;
    }
    .status.pending { background: #fff3cd; color: #856404; }
    .status.connecting { background: #cce5ff; color: #004085; }
    .status.connected { background: #d4edda; color: #155724; }
    .status.scan-detected { background: #e7f3ff; color: #0066cc; }
    .status.error { background: #f8d7da; color: #721c24; }
    .status-icon {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: currentColor;
      animation: pulse 2s infinite;
    }
    @keyframes pulse {
      0%, 100% { opacity: 1; }
      50% { opacity: 0.5; }
    }
    .note {
      margin-top: 20px;
      color: #999;
      font-size: 12px;
      line-height: 1.6;
    }
    .copy-hint {
      font-size: 11px;
      color: #999;
      margin-top: 5px;
    }
    .share-section {
      margin-top: 20px;
      padding-top: 20px;
      border-top: 1px solid #eee;
    }
    .share-btn {
      background: #07c160;
      color: white;
      border: none;
      padding: 10px 24px;
      border-radius: 20px;
      cursor: pointer;
      font-size: 14px;
      transition: opacity 0.2s;
    }
    .share-btn:hover { opacity: 0.9; }
    .share-btn:disabled { opacity: 0.5; cursor: not-allowed; }
    .toast {
      position: fixed;
      bottom: 20px;
      left: 50%;
      transform: translateX(-50%);
      background: rgba(0,0,0,0.8);
      color: white;
      padding: 12px 24px;
      border-radius: 24px;
      font-size: 14px;
      opacity: 0;
      transition: opacity 0.3s;
      pointer-events: none;
    }
    .toast.show { opacity: 1; }
    .switch-notice {
      background: #fff3cd;
      color: #856404;
      padding: 10px 16px;
      border-radius: 8px;
      margin: 10px 0;
      font-size: 13px;
      display: none;
    }
    .switch-notice.show { display: block; }
    .current-user {
      background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
      color: white;
      padding: 12px 20px;
      border-radius: 8px;
      margin-bottom: 20px;
      font-size: 14px;
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 8px;
    }
    .current-user.empty {
      background: #f0f0f0;
      color: #999;
    }
    .user-list {
      margin-top: 10px;
      padding: 10px;
      background: #f8f9fa;
      border-radius: 8px;
      font-size: 12px;
      color: #666;
      max-height: 100px;
      overflow-y: auto;
    }
    .user-list-title { font-weight: 500; margin-bottom: 5px; color: #333; }
  </style>
</head>
<body>
  <div class="container">
    <h1>🤖 微信扫码登录</h1>
    <p class="subtitle">多人可通过此二维码接入同一个 Bot</p>

    <div class="current-user ${state.currentUser ? '' : 'empty'}" id="currentUser">
      <span>👤</span>
      <span>${state.currentUser ? '当前登录: ' + state.currentUser : '暂无用户登录'}</span>
    </div>

    <div class="switch-notice" id="switchNotice">
      🔔 检测到新用户扫码，确认后将切换登录用户
    </div>

    <div class="qr-container">
      ${state.qrcodeUrl || state.status === 'connected'
        ? '<img src="' + (PUBLIC_BASE_URL || '') + '/qrcode-image" alt="微信登录二维码" class="qr-image" id="qrImage">'
        : '<div class="qr-placeholder">等待二维码生成...</div>'
      }
    </div>

    <div id="status" class="status ${state.scanDetected ? 'scan-detected' : state.status}">
      <span class="status-icon"></span>
      <span id="statusText">${getStatusText()}</span>
    </div>

    ${(state.qrcodeUrl || state.status === 'connected') ? `
    <div class="qrcode-url" onclick="copyUrl()" title="点击复制">
      ${(PUBLIC_BASE_URL || '') + '/qrcode-image'}
    </div>
    <div class="copy-hint">↑ 点击复制链接分享给其他人扫码</div>
    ` : ''}

    <div class="share-section">
      <button class="share-btn" onclick="shareQRCode()" ${!state.qrcodeUrl && state.status !== 'connected' ? 'disabled' : ''}>
        📤 分享给其他人扫码
      </button>
    </div>

    <p class="note">
      💡 <strong>使用说明：</strong><br>
      • 二维码长期有效，可多人同时查看<br>
      • 新用户扫码后，当前登录用户会被切换<br>
      • 所有用户发送的消息都会交给 Bot 处理<br>
      • 分享此页面链接给他人即可多人使用
    </p>
  </div>

  <div class="toast" id="toast"></div>

  <script>
    let lastStatus = '${state.status}';
    let scanDetected = ${state.scanDetected};
    let currentUser = '${state.currentUser || ''}';

    function getStatusText() {
      if (scanDetected) return '已检测到扫码，请在手机上确认登录';
      switch (lastStatus) {
        case 'pending': return '准备中...';
        case 'connecting': return '等待扫码...';
        case 'connected': return currentUser ? '✅ ' + currentUser + ' 已登录' : '✅ 登录成功！';
        case 'disconnected': return '❌ 连接断开';
        default: return '未知状态';
      }
    }

    function showToast(message) {
      const toast = document.getElementById('toast');
      toast.textContent = message;
      toast.classList.add('show');
      setTimeout(() => toast.classList.remove('show'), 3000);
    }

    function copyUrl() {
      navigator.clipboard.writeText(window.location.href).then(() => {
        showToast('链接已复制到剪贴板');
      }).catch(() => {
        showToast('复制失败，请手动复制');
      });
    }

    function shareQRCode() {
      const shareData = {
        title: '微信扫码登录 Bot',
        text: '扫描下方二维码登录微信 Bot，多人可同时使用',
        url: window.location.href
      };

      if (navigator.share) {
        navigator.share(shareData).catch(() => {});
      } else {
        copyUrl();
      }
    }

    // Auto-refresh QR image every 5 seconds
    const publicBaseUrl = '${PUBLIC_BASE_URL || ''}';
    setInterval(() => {
      const img = document.getElementById('qrImage');
      if (img) {
        img.src = publicBaseUrl + '/qrcode-image?t=' + Date.now();
      }
    }, 5000);

    // Connect to SSE for real-time updates
    const evtSource = new EventSource('/events');
    evtSource.onmessage = (event) => {
      const data = JSON.parse(event.data);
      console.log('[SSE] Status update:', data);

      lastStatus = data.status;
      scanDetected = data.scanDetected;
      currentUser = data.currentUser || '';

      // Update status display
      const statusEl = document.getElementById('status');
      const statusText = document.getElementById('statusText');
      const currentUserEl = document.getElementById('currentUser');
      const switchNotice = document.getElementById('switchNotice');

      statusEl.className = 'status ' + (data.scanDetected ? 'scan-detected' : data.status);
      statusText.textContent = getStatusText();

      // Update current user display
      if (currentUserEl) {
        if (data.currentUser) {
          currentUserEl.innerHTML = '<span>👤</span><span>当前登录: ' + data.currentUser + '</span>';
          currentUserEl.classList.remove('empty');
        } else {
          currentUserEl.innerHTML = '<span>👤</span><span>暂无用户登录</span>';
          currentUserEl.classList.add('empty');
        }
      }

      // Show switch notice when new scan detected while already connected
      if (data.scanDetected && data.status === 'connected') {
        switchNotice.classList.add('show');
      } else {
        switchNotice.classList.remove('show');
      }

      // Reload page if QR code becomes available
      if (data.qrcodeUrl && !document.getElementById('qrImage')) {
        window.location.reload();
      }

      // Show message when user switches
      if (data.status === 'connected' && data.currentUser) {
        showToast('🎉 ' + data.currentUser + ' 登录成功！');
      }
    };

    evtSource.onerror = () => {
      console.log('[SSE] Connection lost, retrying...');
    };
  </script>
</body>
</html>`;

    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.send(html);
  } else {
    res.json({
      success: true,
      qrcodeUrl: state.qrcodeUrl,
      status: state.status,
      expiresIn: state.status === 'connecting' ? 480 : 0,
    });
  }
});

// Helper function for status text
function getStatusText(): string {
  if (state.scanDetected) return '已检测到扫码，请在手机上确认登录';
  switch (state.status) {
    case 'pending': return '准备中...';
    case 'connecting': return '等待扫码...';
    case 'connected': return '登录成功！';
    case 'disconnected': return '连接断开';
    default: return '未知状态';
  }
}

// QR code plain text endpoint
app.get('/qrcode.txt', (_req: Request, res: Response) => {
  if (state.qrcodeUrl) {
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.send(state.qrcodeUrl);
  } else {
    res.status(404).send('二维码未就绪，请稍后再试');
  }
});

// Status endpoint
app.get('/status', (_req: Request, res: Response) => {
  // Use public base URL for qrcodeUrl if available
  const publicQrUrl = PUBLIC_BASE_URL ? `${PUBLIC_BASE_URL}/qrcode-image` : state.qrcodeUrl;

  res.json({
    status: state.status,
    loggedInAt: state.loggedInAt?.toISOString() || null,
    lastError: state.lastError,
    botId: MEMOH_BOT_ID || null,
    qrcodeUrl: publicQrUrl,
    scanDetected: state.scanDetected,
    confirmationPending: state.confirmationPending,
    currentUser: state.currentUser,
    loginHistory: state.loginHistory,
  });
});

// Logout endpoint - allows new users to scan QR code
app.post('/logout', async (_req: Request, res: Response) => {
  try {
    // Clear credentials file to force re-login
    const fs = await import('fs/promises');
    try {
      await fs.access(TOKEN_PATH);
      await fs.unlink(TOKEN_PATH);
      console.log('[Bridge] Credentials cleared, forcing re-login');
    } catch {
      // File doesn't exist, that's fine
    }

    // Reset state
    state.status = 'connecting';
    state.qrcodeUrl = null;
    state.qrcodeData = null;
    state.currentUser = null;
    state.scanDetected = false;
    state.confirmationPending = false;
    state.loggedInAt = null;

    res.json({
      success: true,
      message: '已退出登录，请刷新页面获取新的二维码',
    });

    // Exit process to force container restart (Docker will restart it)
    setTimeout(() => {
      console.log('[Bridge] Exiting to force restart...');
      process.exit(0);
    }, 1000);
  } catch (error) {
    console.error('[Bridge] Logout error:', error);
    res.status(500).json({
      success: false,
      error: '退出登录失败: ' + (error instanceof Error ? error.message : String(error)),
    });
  }
});

// Validate configuration
function validateConfig(): boolean {
  if (!MEMOH_BOT_ID) {
    console.error('Error: MEMOH_BOT_ID environment variable is required');
    return false;
  }
  if (!MEMOH_API_KEY) {
    console.error('Error: MEMOH_API_KEY environment variable is required');
    return false;
  }
  return true;
}

// Poll for reply from Memoh server
async function pollReply(taskId: string, sender: string, chatId: string): Promise<string | null> {
  const pollUrl = `${MEMOH_SERVER_URL}/channels/weixin/webhook/${MEMOH_BOT_ID}/poll?api_key=${encodeURIComponent(MEMOH_API_KEY)}`;

  const maxAttempts = 60;
  const pollInterval = 2000;

  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    try {
      const response = await fetch(pollUrl);
      if (!response.ok) {
        console.error(`Poll failed: ${response.status}`);
        await new Promise(r => setTimeout(r, pollInterval));
        continue;
      }

      const data = await response.json() as { success: boolean; messages: PendingMessage[] };

      if (data.success && data.messages && data.messages.length > 0) {
        const reply = data.messages.find((m: PendingMessage) => m.task_id === taskId);
        if (reply) {
          return reply.reply;
        }
      }

      await new Promise(r => setTimeout(r, pollInterval));
    } catch (error) {
      console.error('Error polling for reply:', error);
      await new Promise(r => setTimeout(r, pollInterval));
    }
  }

  return null;
}

// Forward message to Memoh and get reply
async function forwardToMemoh(msg: IncomingMessage): Promise<string | null> {
  console.log(`[forwardToMemoh] Starting forward for message: ${msg.text.slice(0, 50)}...`);
  const webhookUrl = `${MEMOH_SERVER_URL}/channels/weixin/webhook/${MEMOH_BOT_ID}`;
  console.log(`[forwardToMemoh] Webhook URL: ${webhookUrl}`);

  const payload = {
    api_key: MEMOH_API_KEY,
    message: msg.text,
    sender: msg.userId,
    sender_name: msg.userId,
    chat_id: msg.userId,
    chat_type: 'private',
    message_id: `wx_${Date.now()}`,
  };

  try {
    // Add timeout for fetch
    const controller = new AbortController();
    const timeoutId = setTimeout(() => {
      console.error('[forwardToMemoh] Fetch timeout triggered after 30s, aborting...');
      controller.abort();
    }, 30000); // 30 second timeout

    console.log('[forwardToMemoh] Calling fetch...');
    const response = await fetch(webhookUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
      signal: controller.signal,
    });
    console.log(`[forwardToMemoh] Fetch returned status: ${response.status}`);

    clearTimeout(timeoutId);

    if (!response.ok) {
      const errorText = await response.text();
      console.error(`Webhook failed: ${response.status} - ${errorText}`);
      return null;
    }

    console.log('[forwardToMemoh] Parsing response JSON...');
    const data = await response.json() as { success: boolean; task_id: string; error?: string };
    console.log(`[forwardToMemoh] Got task_id: ${data.task_id}, success: ${data.success}`);

    if (!data.success) {
      console.error(`Webhook error: ${data.error || 'Unknown error'}`);
      return null;
    }

    return await pollReply(data.task_id, msg.userId, msg.userId);
  } catch (error) {
    console.error('Error forwarding to Memoh:', error);
    return null;
  }
}

// Intercept stdout and stderr to capture QR code URL from SDK
function setupQrCapture(): () => string | null {
  let currentQrUrl: string | null = null;
  const originalStderrWrite = process.stderr.write;
  const originalStdoutWrite = process.stdout.write;

  // Helper to process text for QR code patterns
  const processText = (text: string) => {
    // Log all output containing keywords for debugging (use original write to avoid recursion)
    if (text.includes('http') || text.includes('login') || text.includes('QR') || text.includes('扫码') || text.includes('微信')) {
      originalStdoutWrite.call(process.stdout, `[QR Debug] ${text.slice(0, 200)}\n`, 'utf8');
    }

    // Capture QR code URL pattern - support multiple formats
    // Format 1: Standard http(s) URLs (exclude localhost/local addresses)
    const urlMatch = text.match(/(https?:\/\/[^\s]+)/);
    if (urlMatch) {
      const url = urlMatch[1];
      // Skip local/localhost URLs
      if (url.includes('localhost') || url.includes('127.0.0.1') || url.includes('192.168.') || url.includes('10.0.')) {
        // Ignore local URLs (they are from our own debug output)
      }
      // Check if it looks like a WeChat login URL
      else if (url.includes('ilink') ||
          url.includes('weixin.qq.com') ||
          url.includes('liteapp.weixin.qq.com') ||
          url.includes('login') ||
          url.includes('qr') ||
          url.includes('qrcode') ||
          text.includes('微信') ||
          text.includes('扫码') ||
          text.includes('QR') ||
          text.includes('登录')) {
        currentQrUrl = url;
        state.qrcodeUrl = currentQrUrl;
        state.qrcodeData = currentQrUrl;
        state.status = 'connecting';
        originalStdoutWrite.call(process.stdout, `[Bridge] QR code URL captured: ${currentQrUrl.substring(0, 50)}...\n`, 'utf8');
        broadcastState();
      }
    }

    // Format 2: WeChat protocol URLs (weixin://)
    const weixinProtocolMatch = text.match(/(weixin:\/\/[^\s]+)/);
    if (weixinProtocolMatch) {
      currentQrUrl = weixinProtocolMatch[1];
      state.qrcodeUrl = currentQrUrl;
      state.qrcodeData = currentQrUrl;
      state.status = 'connecting';
      originalStdoutWrite.call(process.stdout, `[Bridge] WeChat protocol URL captured: ${currentQrUrl}\n`, 'utf8');
      broadcastState();
    }

    // Capture status changes
    if (text.includes('Logged in as') || text.includes('登录成功') || text.includes('已登录')) {
      // Extract user name from login message if possible
      const userMatch = text.match(/Logged in as\s+(.+?)(?:\s|$)/);
      const userName = userMatch ? userMatch[1].trim() : '未知用户';

      state.status = 'connected';
      state.loggedInAt = new Date();
      state.currentUser = userName;

      // Add to login history (keep last 10)
      if (!state.loginHistory.includes(userName)) {
        state.loginHistory.push(userName);
        if (state.loginHistory.length > 10) {
          state.loginHistory.shift();
        }
      }

      // Note: We keep qrcodeUrl and qrcodeData so others can still scan
      // When a new user scans, they will take over the login
      state.scanDetected = false;
      state.confirmationPending = false;
      originalStdoutWrite.call(process.stdout, `[Bridge] Login successful! User: ${userName}\n`, 'utf8');
      broadcastState();
    }

    // Handle new scan from different user (for multi-user access)
    if (text.includes('New scan detected') || text.includes('新扫码') || (text.includes('扫码') && state.status === 'connected')) {
      state.scanDetected = true;
      state.confirmationPending = true;
      originalStdoutWrite.call(process.stdout, '[Bridge] New user scanning QR code, preparing to switch login...\n', 'utf8');
      broadcastState();
    }

    if (text.includes('QR code scanned') || text.includes('扫码成功') || text.includes('已扫码') || text.includes('scan')) {
      state.scanDetected = true;
      state.confirmationPending = true;
      originalStdoutWrite.call(process.stdout, '[Bridge] QR code scanned, waiting for confirmation...\n', 'utf8');
      broadcastState();
    }

    if (text.includes('Login confirmed') || text.includes('确认登录') || text.includes('confirmed')) {
      state.confirmationPending = false;
      originalStdoutWrite.call(process.stdout, '[Bridge] Login confirmed!\n', 'utf8');
      broadcastState();
    }

    if (text.includes('disconnected') || text.includes('断开') || text.includes('logout')) {
      state.status = 'disconnected';
      originalStdoutWrite.call(process.stdout, '[Bridge] Disconnected\n', 'utf8');
      broadcastState();
    }
  };

  // Intercept stderr
  process.stderr.write = function(
    chunk: Uint8Array | string,
    encoding?: BufferEncoding | ((err?: Error | null) => void),
    callback?: (err?: Error | null) => void,
  ): boolean {
    const text = typeof chunk === 'string' ? chunk : chunk.toString();
    processText(text);

    // Call original write
    if (typeof encoding === 'function') {
      callback = encoding;
      encoding = undefined;
    }
    return originalStderrWrite.call(process.stderr, chunk, encoding as BufferEncoding, callback);
  };

  // Intercept stdout
  process.stdout.write = function(
    chunk: Uint8Array | string,
    encoding?: BufferEncoding | ((err?: Error | null) => void),
    callback?: (err?: Error | null) => void,
  ): boolean {
    const text = typeof chunk === 'string' ? chunk : chunk.toString();
    processText(text);

    // Call original write
    if (typeof encoding === 'function') {
      callback = encoding;
      encoding = undefined;
    }
    return originalStdoutWrite.call(process.stdout, chunk, encoding as BufferEncoding, callback);
  };

  return () => currentQrUrl;
}

// Main function
async function main() {
  console.log('=== WeChat Personal Account Bridge for Memoh ===');
  console.log('Using @pinixai/weixin-bot SDK');

  if (!validateConfig()) {
    process.exit(1);
  }

  console.log('\nConfiguration:');
  console.log(`  MEMOH_SERVER_URL: ${MEMOH_SERVER_URL}`);
  console.log(`  MEMOH_BOT_ID: ${MEMOH_BOT_ID}`);
  console.log(`  API_KEY: ${MEMOH_API_KEY.slice(0, 10)}...`);
  console.log(`  TOKEN_PATH: ${TOKEN_PATH}`);

  // Setup QR code capture
  setupQrCapture();

  // Create bot instance
  const bot = new WeixinBot({
    tokenPath: TOKEN_PATH,
    onError: (error) => {
      console.error('[Bot Error]', error);
      state.lastError = error instanceof Error ? error.message : String(error);
      broadcastState();
    },
  });

  // Register message handler
  bot.onMessage(async (msg: IncomingMessage) => {
    console.log(`[Message] From ${msg.userId}: ${msg.text.slice(0, 100)}...`);
    console.log(`[onMessage] Starting processing...`);

    try {
      console.log(`[onMessage] Calling forwardToMemoh...`);
      // Forward to Memoh and get reply
      const reply = await forwardToMemoh(msg);
      console.log(`[onMessage] forwardToMemoh returned: ${reply ? reply.slice(0, 50) : 'null'}`);

      if (reply) {
        console.log(`[Reply] ${reply.slice(0, 100)}...`);
        await bot.reply(msg, reply);
      } else {
        await bot.reply(msg, '抱歉，我暂时无法处理这条消息，请稍后再试。');
      }
    } catch (error) {
      console.error('Error handling message:', error);
      try {
        await bot.reply(msg, '处理消息时出错了，请稍后再试。');
      } catch (replyError) {
        console.error('Error sending error reply:', replyError);
      }
    }
  });

  // Start HTTP control server
  const server = app.listen(BRIDGE_PORT, () => {
    console.log(`\nControl server listening on port ${BRIDGE_PORT}`);
    console.log(`  - Health: http://localhost:${BRIDGE_PORT}/health`);
    console.log(`  - QR Code: http://localhost:${BRIDGE_PORT}/qrcode`);
    console.log(`  - QR Image: http://localhost:${BRIDGE_PORT}/qrcode-image`);
    console.log(`  - Status: http://localhost:${BRIDGE_PORT}/status`);
    console.log(`  - Events: http://localhost:${BRIDGE_PORT}/events (SSE)`);
  });

  // Login and start message loop
  console.log('\nStarting WeChat login process...');
  state.status = 'connecting';

  try {
    await bot.login();
    console.log('\n✅ WeChat login successful!');

    console.log('\nStarting message loop...');
    await bot.run();
  } catch (error) {
    state.status = 'disconnected';
    state.lastError = error instanceof Error ? error.message : String(error);
    console.error('Fatal error:', error);
    server.close();
    process.exit(1);
  }
}

// Handle graceful shutdown
process.on('SIGTERM', () => {
  console.log('Received SIGTERM, shutting down gracefully...');
  process.exit(0);
});

process.on('SIGINT', () => {
  console.log('Received SIGINT, shutting down gracefully...');
  process.exit(0);
});

// Run main
main().catch(err => {
  console.error('Unhandled error:', err);
  process.exit(1);
});
