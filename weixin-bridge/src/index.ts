import express, { type Request, type Response } from 'express';
import { WeixinBot, type IncomingMessage } from '@pinixai/weixin-bot';

interface PendingMessage {
  task_id: string;
  reply: string;
  sender: string;
  chat_id: string;
}

interface BridgeState {
  status: 'pending' | 'connecting' | 'connected' | 'disconnected';
  qrcodeUrl: string | null;
  lastError: string | null;
  loggedInAt: Date | null;
}

// Configuration from environment
const MEMOH_SERVER_URL = process.env.MEMOH_SERVER_URL || 'http://server:8080';
const MEMOH_BOT_ID = process.env.MEMOH_BOT_ID || '';
const MEMOH_API_KEY = process.env.MEMOH_API_KEY || '';
const BRIDGE_PORT = parseInt(process.env.BRIDGE_PORT || '3000');
const TOKEN_PATH = process.env.TOKEN_PATH || '/data/.weixin-bot/credentials.json';

// Bridge state
const state: BridgeState = {
  status: 'pending',
  qrcodeUrl: null,
  lastError: null,
  loggedInAt: null,
};

// Express app for control endpoints
const app = express();
app.use(express.json());

// Health check endpoint
app.get('/health', (_req: Request, res: Response) => {
  res.json({
    status: state.status,
    timestamp: new Date().toISOString(),
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
  <title>微信扫码登录</title>
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
      margin: 0;
      background: #f5f5f5;
    }
    .container {
      background: white;
      padding: 40px;
      border-radius: 12px;
      box-shadow: 0 4px 20px rgba(0,0,0,0.1);
      text-align: center;
      max-width: 600px;
    }
    h1 { color: #333; margin-bottom: 10px; }
    p { color: #666; margin-bottom: 30px; }
    .qrcode-url {
      word-break: break-all;
      background: #f0f0f0;
      padding: 15px;
      border-radius: 8px;
      font-family: monospace;
      font-size: 14px;
      margin: 20px 0;
    }
    .status {
      margin-top: 20px;
      padding: 10px 20px;
      border-radius: 20px;
      background: #e3f2fd;
      color: #1976d2;
      font-size: 14px;
    }
    .note {
      margin-top: 20px;
      color: #999;
      font-size: 12px;
    }
  </style>
</head>
<body>
  <div class="container">
    <h1>微信扫码登录</h1>
    <p>请使用微信打开下方链接完成登录</p>
    <div class="qrcode-url">${state.qrcodeUrl || '等待二维码生成...'}</div>
    <div class="status">状态: ${state.status} | 点击链接后请在微信中确认登录</div>
    <p class="note">注意：微信内打开链接后需要点击"确认登录"</p>
  </div>
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
  res.json({
    status: state.status,
    loggedInAt: state.loggedInAt?.toISOString() || null,
    lastError: state.lastError,
    botId: MEMOH_BOT_ID || null,
    qrcodeUrl: state.qrcodeUrl,
  });
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

// Intercept stderr to capture QR code URL from SDK
function setupQrCapture(): () => string | null {
  let currentQrUrl: string | null = null;
  const originalWrite = process.stderr.write;

  process.stderr.write = function(
    chunk: Uint8Array | string,
    encoding?: BufferEncoding | ((err?: Error | null) => void),
    callback?: (err?: Error | null) => void
  ): boolean {
    const text = typeof chunk === 'string' ? chunk : chunk.toString();

    // Capture QR code URL pattern (http:// or https://)
    const urlMatch = text.match(/(https?:\/\/[^\s]+)/);
    if (urlMatch && (text.includes('ilink') || text.includes('微信') || text.includes('login'))) {
      currentQrUrl = urlMatch[1];
      state.qrcodeUrl = currentQrUrl;
      console.log(`[Bridge] QR code URL captured: ${currentQrUrl.substring(0, 50)}...`);
    }

    // Capture status changes
    if (text.includes('Logged in as')) {
      state.status = 'connected';
      state.loggedInAt = new Date();
      state.qrcodeUrl = null;
    }

    if (text.includes('QR code scanned')) {
      console.log('[Bridge] QR code scanned, waiting for confirmation...');
    }

    if (text.includes('Login confirmed')) {
      console.log('[Bridge] Login confirmed!');
    }

    // Call original write
    if (typeof encoding === 'function') {
      callback = encoding;
      encoding = undefined;
    }
    return originalWrite.call(process.stderr, chunk, encoding as BufferEncoding, callback);
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
    console.log(`  - Status: http://localhost:${BRIDGE_PORT}/status`);
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
