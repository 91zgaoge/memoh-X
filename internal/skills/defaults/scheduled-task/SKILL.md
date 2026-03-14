---
name: scheduled-task
description: Create scheduled tasks for recurring or one-time automated execution. Use when users want to set up tasks that run automatically at specified times (daily, weekly, monthly, cron, or one-time).
---

# Scheduled Task Skill

## Usage Scenarios

Use this skill when users want to:
- Set up tasks that run on a schedule (daily, weekly, monthly, custom cron)
- Create one-time tasks that run at a specific time
- Schedule automated checks, report generation, code backups, etc.
- Set up periodic monitoring or reminders

## Creating Scheduled Tasks

### Step 1: Gather Information

Confirm the following with the user (if not provided):
1. **Task name** (required) — Short description
2. **Execution content** (required) — The prompt/instructions Claude receives when the task runs
3. **Execution frequency** (required) — One-time, daily, weekly, monthly, or custom cron
4. **Working directory** (optional) — Defaults to current session working directory
5. **Notification platforms** (optional) — Send notifications after task completion

### Step 2: Build JSON and Execute API Call

#### Schedule Types

**One-time execution (at):**
```json
{ "type": "at", "datetime": "2026-03-15T09:00:00" }
```

**Cron expression (cron) — 5-field format: minute hour day month weekday**
```json
{ "type": "cron", "expression": "0 9 * * *" }
```

Common cron examples:
| Expression | Meaning |
|--------|------|
| `0 9 * * *` | Every day at 9:00 AM |
| `0 8 * * 1` | Every Monday at 8:00 AM |
| `0 9 * * 1-5` | Weekdays at 9:00 AM |
| `0 0 1 * *` | First day of month at midnight |
| `*/30 * * * *` | Every 30 minutes |
| `0 * * * *` | Every hour on the hour |
| `0 9,18 * * *` | Every day at 9:00 AM and 6:00 PM |

#### Create Task via API

Use the backend API to create scheduled tasks. The API endpoint should support the following payload structure:

```json
{
  "name": "Task name",
  "schedule": { "type": "cron", "expression": "0 9 * * *" },
  "prompt": "Detailed instructions Claude will execute when task runs...",
  "workingDirectory": "/path/to/project",
  "description": "Optional detailed description",
  "systemPrompt": "Optional custom system prompt",
  "executionMode": "auto",
  "expiresAt": "2026-12-31",
  "notifyPlatforms": ["dingtalk", "feishu", "telegram", "discord"],
  "enabled": true
}
```

#### Field Descriptions

| Field | Required | Description |
|------|------|------|
| `name` | ✅ | Short task name |
| `prompt` | ✅ | Instructions Claude receives when task runs (should be clear and complete) |
| `schedule` | ✅ | Schedule configuration (see types above) |
| `workingDirectory` | ❌ | Execution directory (defaults to empty) |
| `description` | ❌ | Detailed description (defaults to empty) |
| `systemPrompt` | ❌ | Custom system prompt (defaults to empty) |
| `executionMode` | ❌ | `"auto"` / `"local"` / `"sandbox"` (defaults to `"local"`) |
| `expiresAt` | ❌ | Expiration date `"YYYY-MM-DD"` (defaults to null, no expiration) |
| `notifyPlatforms` | ❌ | Notification platform array: `["dingtalk","feishu","telegram","discord"]` (defaults to `[]`) |
| `enabled` | ❌ | Whether to enable immediately (defaults to `true`) |

### Step 3: Confirm Results

API returns JSON response:
- Success: `{ "success": true, "task": { "id": "...", "name": "...", ... } }`
- Failure: `{ "success": false, "error": "error message" }`

Confirm the following with the user:
- ✅ Task name and ID
- ⏰ Execution frequency (human-readable format, e.g., "Every day at 9:00 AM")
- 📋 Execution content summary
- 💡 Remind user they can manage tasks in Settings → Scheduled Tasks

## Important Notes

- **Critical: Timezone Awareness**: The system has a configured timezone (e.g., 'Asia/Shanghai'). ALWAYS get current time in THIS timezone for calculations. Using wrong timezone will cause schedules to fire at wrong times.

- **One-time vs Recurring Tasks**: This is CRITICAL - ask yourself: does the user want this to happen once or repeatedly?
  - **One-time tasks** (specific date/time mentioned like "明天下午5点去机场", "后天上午9点开会"): Calculate the EXACT date, use format `0 17 DD MM *`, AND set `max_calls: 1`
  - **Recurring tasks** (words like "每天", "每周", "each day", "every week"): Use standard cron like `0 17 * * *`, do NOT set max_calls
  - When in doubt, ask the user if this is a one-time or recurring task

- **Natural language time conversion**: When users specify times like "下午5点/5 PM", "明天上午9点", "5分钟后", "this afternoon":
  1. **Get current time in system timezone** using a system command with TZ set (e.g., `TZ=Asia/Shanghai date` or Node.js with timezone)
  2. **Calculate the exact target time** based on current time in that timezone
  3. **Convert to cron expression**:
     - "下午5点/17:00 每天" (recurring) → `0 17 * * *` (no max_calls)
     - "明天下午5点去机场" (one-time) → Calculate tomorrow's date, use `0 17 DD MM *` + `max_calls: 1`
     - "5分钟后" → Calculate target minute, use `MM HH * * *` + `max_calls: 1`
  4. **DO NOT** interpret "5点" as "5 minutes" - "点" means "o'clock" in Chinese
- **Creation timing**: For short-delay one-time tasks (e.g., "in 1 minute"), create the task immediately before performing any time-consuming operations. Don't fetch data or summarize content before creating the task.
- **Prompt boundaries**: The `prompt` should describe "what to do when the task triggers", not pre-execute the task and embed static results. Example: write "Fetch yesterday's AI news and send summary" instead of fetching news first and embedding the list in the prompt.
- **Get current time** (cross-platform):
  ```bash
  node -e 'const d=new Date();const p=n=>String(n).padStart(2,"0");console.log(`${d.getFullYear()}-${p(d.getMonth()+1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`)'
  ```
- **Auto-execution**: Scheduled tasks run with auto-approve enabled for all tool calls (no manual approval needed)
- **Independent execution**: The `prompt` is the only instruction Claude receives when the task runs independently, so write it clearly and completely
- **Auto-disable**: Tasks that fail 5 consecutive times are automatically disabled
- **One-time tasks**: Tasks with `type: "at"` are automatically disabled after execution
- **Execution sessions**: Each execution creates a new session (with "[Scheduled]" prefix in title), viewable in the session list
