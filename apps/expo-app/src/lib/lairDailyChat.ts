import * as SQLite from "expo-sqlite";
import type { EventEnvelope } from "../types/contracts";
import { firstTaskErrorMessage } from "./runtimeEvents";

export type LairDailyChatRole = "user" | "milo" | "system";

export type LairDailyChatMessage = {
  id: string;
  chat_date: string;
  role: LairDailyChatRole;
  text: string;
  task_id: string | null;
  attempt_id: string | null;
  source_event_type: string | null;
  created_at: string;
};

export type PendingChatMessage = {
  id?: string;
  role: LairDailyChatRole;
  text: string;
  task_id?: string | null;
  attempt_id?: string | null;
  source_event_type?: string | null;
  created_at?: string;
};

let dbPromise: Promise<SQLite.SQLiteDatabase> | null = null;

async function getDb() {
  if (!dbPromise) {
    dbPromise = SQLite.openDatabaseAsync("xmilo_archive.db");
  }
  return dbPromise;
}

export function localChatDate(date = new Date()) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export async function initLairDailyChat() {
  const db = await getDb();
  await db.execAsync(`
    PRAGMA journal_mode = WAL;
    CREATE TABLE IF NOT EXISTS lair_daily_chat_messages (
      id TEXT PRIMARY KEY,
      chat_date TEXT NOT NULL,
      role TEXT NOT NULL,
      text TEXT NOT NULL,
      task_id TEXT,
      attempt_id TEXT,
      source_event_type TEXT,
      created_at TEXT NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_lair_daily_chat_date_created
      ON lair_daily_chat_messages(chat_date, created_at);
  `);
}

export async function listTodayLairChatMessages(chatDate = localChatDate()) {
  await initLairDailyChat();
  await clearOldLairChatMessages(chatDate);
  const db = await getDb();
  return db.getAllAsync<LairDailyChatMessage>(
    `SELECT id, chat_date, role, text, task_id, attempt_id, source_event_type, created_at
     FROM lair_daily_chat_messages
     WHERE chat_date = ?
     ORDER BY created_at ASC`,
    chatDate
  );
}

export async function appendLairChatMessage(message: PendingChatMessage, chatDate = localChatDate()) {
  await initLairDailyChat();
  const db = await getDb();
  const createdAt = message.created_at ?? new Date().toISOString();
  const id = message.id ?? `chat:${createdAt}:${message.role}:${sanitizeToken(message.task_id ?? "")}:${sanitizeToken(message.source_event_type ?? "")}`;
  const safeMessage: LairDailyChatMessage = {
    id,
    chat_date: chatDate,
    role: message.role,
    text: sanitizeChatText(message.text),
    task_id: sanitizeNullableToken(message.task_id),
    attempt_id: sanitizeNullableToken(message.attempt_id),
    source_event_type: sanitizeNullableToken(message.source_event_type),
    created_at: createdAt,
  };

  if (!safeMessage.text) {
    return null;
  }

  await db.runAsync(
    `INSERT OR IGNORE INTO lair_daily_chat_messages
      (id, chat_date, role, text, task_id, attempt_id, source_event_type, created_at)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
    safeMessage.id,
    safeMessage.chat_date,
    safeMessage.role,
    safeMessage.text,
    safeMessage.task_id,
    safeMessage.attempt_id,
    safeMessage.source_event_type,
    safeMessage.created_at
  );
  return safeMessage;
}

export async function clearLairChatForDate(chatDate = localChatDate()) {
  await initLairDailyChat();
  const db = await getDb();
  await db.runAsync(`DELETE FROM lair_daily_chat_messages WHERE chat_date = ?`, chatDate);
}

export async function updateLairChatMessageTaskIdentity(id: string, taskId?: string | null, attemptId?: string | null) {
  await initLairDailyChat();
  const safeId = sanitizeToken(id);
  if (!safeId) return;
  const db = await getDb();
  await db.runAsync(
    `UPDATE lair_daily_chat_messages
     SET task_id = COALESCE(?, task_id),
         attempt_id = COALESCE(?, attempt_id)
     WHERE id = ?`,
    sanitizeNullableToken(taskId),
    sanitizeNullableToken(attemptId),
    safeId
  );
}

export async function removeLairChatMessagesForTaskSource(taskId: string | null | undefined, sourceEventType: string) {
  await initLairDailyChat();
  const safeTaskId = sanitizeNullableToken(taskId);
  const safeSource = sanitizeNullableToken(sourceEventType);
  if (!safeTaskId || !safeSource) return;
  const db = await getDb();
  await db.runAsync(
    `DELETE FROM lair_daily_chat_messages WHERE task_id = ? AND source_event_type = ?`,
    safeTaskId,
    safeSource
  );
}

export async function clearOldLairChatMessages(today = localChatDate()) {
  await initLairDailyChat();
  const db = await getDb();
  await db.runAsync(`DELETE FROM lair_daily_chat_messages WHERE chat_date <> ?`, today);
}

export async function resetLairChatAfterNightlyArchive(archiveDate?: string | null) {
  await initLairDailyChat();
  const db = await getDb();
  const safeArchiveDate = typeof archiveDate === "string" && /^\d{4}-\d{2}-\d{2}$/.test(archiveDate) ? archiveDate : "";
  if (safeArchiveDate) {
    await db.runAsync(`DELETE FROM lair_daily_chat_messages WHERE chat_date = ?`, safeArchiveDate);
  }
  await clearOldLairChatMessages(localChatDate());
}

export function chatProjectionFromRuntimeEvent(
  event: EventEnvelope,
  options: { reportReadyTaskIds: Set<string>; currentTaskId?: string }
): PendingChatMessage | null {
  const payload = event.payload ?? {};
  const taskId = sanitizeNullableToken(payload.task_id);
  const attemptId = sanitizeNullableToken(payload.attempt_id);
  const base = {
    id: eventId(event),
    task_id: taskId,
    attempt_id: attemptId,
    source_event_type: event.type,
    created_at: event.timestamp,
  };

  switch (event.type) {
    case "report.ready": {
      const text = sanitizeChatText(payload.report_text);
      if (!text) return null;
      return { ...base, role: "milo", text };
    }
    case "task.message_emitted": {
      const text = sanitizeChatText(payload.message);
      if (!text) return null;
      return { ...base, role: "milo", text };
    }
    case "task.completed": {
      if (taskId && options.reportReadyTaskIds.has(taskId)) return null;
      const summary = sanitizeChatText(payload.summary);
      if (!summary) return null;
      return { ...base, role: "system", text: `Done: ${summary}` };
    }
    case "task.stuck": {
      if (options.currentTaskId && taskId && taskId !== options.currentTaskId) return null;
      const reason = sanitizeChatText(firstTaskErrorMessage(String(payload.reason ?? payload.message ?? "Task needs attention.")));
      return { ...base, role: "system", text: reason || "Task needs attention." };
    }
    case "runtime.error":
    case "ui_local.error": {
      const text = sanitizeChatText(firstTaskErrorMessage(String(payload.message ?? "Milo needs a moment before retrying.")));
      if (!text) return null;
      return { ...base, role: "system", text };
    }
    default:
      return null;
  }
}

function eventId(event: EventEnvelope) {
  const taskId = sanitizeToken(event.payload?.task_id);
  const attemptId = sanitizeToken(event.payload?.attempt_id);
  return `event:${sanitizeToken(event.type)}:${sanitizeToken(event.timestamp)}:${taskId}:${attemptId}`;
}

function sanitizeNullableToken(value: unknown) {
  const token = sanitizeToken(value);
  return token || null;
}

function sanitizeToken(value: unknown) {
  if (typeof value !== "string" && typeof value !== "number" && typeof value !== "boolean") return "";
  return String(value).trim().replace(/[^A-Za-z0-9_.:/-]/g, "_").slice(0, 120);
}

function sanitizeChatText(value: unknown) {
  if (typeof value !== "string") return "";
  return value
    .trim()
    .replace(/eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}/g, "<redacted>")
    .replace(/authorization\s*[:=]\s*bearer\s+[^\s,"'}]+/gi, "authorization: bearer <redacted>")
    .replace(/bearer\s+[A-Za-z0-9._-]{16,}/gi, "bearer <redacted>")
    .replace(/[A-Za-z0-9_-]{48,}/g, "<redacted>")
    .replace(/\s+/g, " ")
    .slice(0, 2000);
}
