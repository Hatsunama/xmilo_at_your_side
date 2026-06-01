import * as FileSystem from "expo-file-system/legacy";
import * as SQLite from "expo-sqlite";
import { hashSettingsReportBundle, redactSettingsReportPayload, type SettingsReportSubmitPayload } from "./reportBundle";
import { submitSettingsReport, type SettingsReportProof } from "./bridge";
import {
  sanitizeSettingsReportFailureClass,
  type SettingsReportFailureClass
} from "./reportFailureClasses";

export type QueuedSettingsReport = {
  client_report_id: string;
  created_at: string;
  status: "queued_offline" | "sent";
  source: "settings_button";
  bundle_uri: string;
  bundle_hash: string;
  comment_present: boolean;
  last_error: string | null;
  failure_class?: SettingsReportFailureClass | null;
  sent_report_id?: string | null;
  relay_report_id?: string | null;
  sent_at?: string | null;
};

export type SettingsReportQueueDrainResult = {
  attempted: number;
  sent: number;
  failed: number;
  proofs: SettingsReportProof[];
};

let dbPromise: Promise<SQLite.SQLiteDatabase> | null = null;

async function getDb() {
  if (!dbPromise) {
    dbPromise = SQLite.openDatabaseAsync("xmilo_archive.db");
  }
  return dbPromise;
}

export async function initSettingsReportQueue() {
  const db = await getDb();
  await db.execAsync(`
    PRAGMA journal_mode = WAL;
    CREATE TABLE IF NOT EXISTS queued_settings_reports (
      client_report_id TEXT PRIMARY KEY,
      created_at TEXT NOT NULL,
      status TEXT NOT NULL,
      source TEXT NOT NULL,
      bundle_uri TEXT NOT NULL,
      bundle_hash TEXT NOT NULL,
      comment_present INTEGER NOT NULL DEFAULT 0,
      last_error TEXT,
      failure_class TEXT,
      sent_report_id TEXT,
      relay_report_id TEXT,
      sent_at TEXT,
      updated_at TEXT NOT NULL
    );
  `);
  await addColumnIfMissing("queued_settings_reports", "sent_report_id", "TEXT");
  await addColumnIfMissing("queued_settings_reports", "relay_report_id", "TEXT");
  await addColumnIfMissing("queued_settings_reports", "sent_at", "TEXT");
  await addColumnIfMissing("queued_settings_reports", "failure_class", "TEXT");
}

export async function enqueueSettingsReport(input: {
  payload: SettingsReportSubmitPayload;
  lastError?: string | null;
  failureClass?: SettingsReportFailureClass | null;
}): Promise<QueuedSettingsReport> {
  await initSettingsReportQueue();
  const directory = await ensureReportDirectory();
  const payload = redactSettingsReportPayload(input.payload);
  payload.bundle_hash = hashSettingsReportBundle(payload.bundle);
  const bundleUri = `${directory}${payload.client_report_id}.json`;
  const serialized = JSON.stringify(payload, null, 2);
  await FileSystem.writeAsStringAsync(bundleUri, serialized, { encoding: FileSystem.EncodingType.UTF8 });

  const queued: QueuedSettingsReport = {
    client_report_id: payload.client_report_id,
    created_at: payload.created_at,
    status: "queued_offline",
    source: "settings_button",
    bundle_uri: bundleUri,
    bundle_hash: payload.bundle_hash,
    comment_present: payload.user_note.trim().length > 0,
    last_error: sanitizeQueueError(input.lastError),
    failure_class: input.failureClass ? sanitizeSettingsReportFailureClass(input.failureClass) : null,
    sent_report_id: null,
    relay_report_id: null,
    sent_at: null,
  };

  const db = await getDb();
  await db.runAsync(
    `INSERT OR REPLACE INTO queued_settings_reports
      (client_report_id, created_at, status, source, bundle_uri, bundle_hash, comment_present, last_error, failure_class, updated_at)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    queued.client_report_id,
    queued.created_at,
    queued.status,
    queued.source,
    queued.bundle_uri,
    queued.bundle_hash,
    queued.comment_present ? 1 : 0,
    queued.last_error,
    queued.failure_class ?? null,
    new Date().toISOString()
  );

  return queued;
}

export async function drainQueuedSettingsReports(options?: { limit?: number }): Promise<SettingsReportQueueDrainResult> {
  await initSettingsReportQueue();
  const db = await getDb();
  const limit = Math.max(1, Math.min(options?.limit ?? 5, 10));
  const rows = await db.getAllAsync<QueuedSettingsReport>(
    `SELECT client_report_id, created_at, status, source, bundle_uri, bundle_hash, comment_present, last_error, failure_class, sent_report_id, relay_report_id, sent_at
     FROM queued_settings_reports
     WHERE status = 'queued_offline'
     ORDER BY created_at ASC
     LIMIT ?`,
    limit
  );
  const result: SettingsReportQueueDrainResult = { attempted: 0, sent: 0, failed: 0, proofs: [] };

  for (const row of rows) {
    result.attempted += 1;
    logSettingsReportQueueBreadcrumb("settings_report_queue_drain_item_attempted");
    try {
      const raw = await FileSystem.readAsStringAsync(row.bundle_uri, { encoding: FileSystem.EncodingType.UTF8 });
      const payload = JSON.parse(raw) as SettingsReportSubmitPayload;
      if (payload.client_report_id !== row.client_report_id || payload.bundle_hash !== row.bundle_hash) {
        throw new Error("queued report metadata mismatch");
      }
      const proof = await submitSettingsReport(payload);
      if (proof.accepted === true && proof.report_id) {
        const sentAt = new Date().toISOString();
        await db.runAsync(
          `UPDATE queued_settings_reports
           SET status = 'sent', sent_report_id = ?, relay_report_id = ?, sent_at = ?, last_error = NULL, failure_class = NULL, updated_at = ?
           WHERE client_report_id = ?`,
          proof.report_id,
          proof.report_id,
          sentAt,
          sentAt,
          row.client_report_id
        );
        result.sent += 1;
        result.proofs.push(proof);
        logSettingsReportQueueBreadcrumb("settings_report_queue_drain_item_sent", { report_id_present: true });
      } else {
        throw new Error("report service did not return accepted proof");
      }
    } catch (error) {
      const failureClass = classifyDrainFailure(error);
      result.failed += 1;
      logSettingsReportQueueBreadcrumb("settings_report_queue_drain_item_failed", { class: failureClass });
      await db.runAsync(
        `UPDATE queued_settings_reports
         SET last_error = ?, failure_class = ?, updated_at = ?
         WHERE client_report_id = ?`,
        sanitizeQueueError((error as Error | undefined)?.message ?? "report service unavailable"),
        failureClass,
        new Date().toISOString(),
        row.client_report_id
      );
    }
  }

  return result;
}

export async function hasQueuedSettingsReports(): Promise<boolean> {
  return (await countQueuedSettingsReports()) > 0;
}

export async function countQueuedSettingsReports(): Promise<number> {
  await initSettingsReportQueue();
  const db = await getDb();
  const row = await db.getFirstAsync<{ count: number }>(
    `SELECT COUNT(*) AS count
     FROM queued_settings_reports
     WHERE status = 'queued_offline'`
  );
  return Number(row?.count ?? 0);
}

async function ensureReportDirectory() {
  if (!FileSystem.documentDirectory) {
    throw new Error("document storage unavailable");
  }
  const directory = `${FileSystem.documentDirectory}settings-report-queue/`;
  await FileSystem.makeDirectoryAsync(directory, { intermediates: true });
  return directory;
}

async function addColumnIfMissing(table: string, column: string, definition: string) {
  const db = await getDb();
  const columns = await db.getAllAsync<{ name: string }>(`PRAGMA table_info(${table})`);
  if (columns.some((item) => item.name === column)) return;
  await db.execAsync(`ALTER TABLE ${table} ADD COLUMN ${column} ${definition}`);
}

function sanitizeQueueError(error: string | null | undefined) {
  if (!error) return null;
  return String(error)
    .replace(/eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}/g, "<redacted>")
    .replace(/authorization\s*[:=]\s*bearer\s+[^\s,"'}]+/gi, "authorization: bearer <redacted>")
    .replace(/bearer\s+[A-Za-z0-9._-]{16,}/gi, "bearer <redacted>")
    .replace(/[A-Za-z0-9_-]{48,}/g, "<redacted>")
    .slice(0, 240);
}

function classifyDrainFailure(error: unknown): SettingsReportFailureClass {
  const text = sanitizeQueueError((error as Error | undefined)?.message ?? String(error ?? ""))?.toLowerCase() ?? "";
  if (text.includes("accepted proof")) return "proof_missing_accepted_or_report_id";
  if (text.includes("report/settings") || text.includes("report service")) return "sidecar_report_submit_failed";
  if (text.includes("relay") || text.includes("bad gateway") || text.includes("502") || text.includes("503")) return "relay_rejected_or_unreachable";
  return "unknown_report_send_failure";
}

function logSettingsReportQueueBreadcrumb(event: string, fields?: Record<string, string | number | boolean | undefined>) {
  const parts = [`xMilo ${event}`];
  for (const [key, value] of Object.entries(fields ?? {})) {
    if (typeof value === "undefined") continue;
    parts.push(`${key}=${String(value)}`);
  }
  console.info(parts.join(" "));
}
