import * as SQLite from "expo-sqlite";

let dbPromise: Promise<SQLite.SQLiteDatabase> | null = null;

async function getDb() {
  if (!dbPromise) {
    dbPromise = SQLite.openDatabaseAsync("xmilo_archive.db");
  }
  return dbPromise;
}

export async function initArchiveDb() {
  const db = await getDb();
  await db.execAsync(`
    PRAGMA journal_mode = WAL;
    CREATE TABLE IF NOT EXISTS archive_records (
      task_id TEXT NOT NULL,
      title TEXT NOT NULL,
      description TEXT NOT NULL,
      created_at TEXT NOT NULL,
      PRIMARY KEY (task_id, created_at)
    );
    CREATE TABLE IF NOT EXISTS app_settings (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL
    );
    CREATE TABLE IF NOT EXISTS staged_inputs (
      id TEXT PRIMARY KEY,
      input_type TEXT NOT NULL,
      label TEXT NOT NULL,
      uri TEXT,
      mime_type TEXT,
      text_excerpt TEXT,
      provenance TEXT NOT NULL,
      created_at TEXT NOT NULL
    );
  `);
}

export async function addArchiveRecord(record: {
  task_id: string;
  title: string;
  description: string;
  created_at: string;
}) {
  const db = await getDb();
  await db.runAsync(
    `INSERT OR REPLACE INTO archive_records(task_id, title, description, created_at) VALUES(?, ?, ?, ?)`,
    record.task_id,
    record.title,
    record.description,
    record.created_at
  );
}

export async function listArchiveRecords() {
  const db = await getDb();
  const rows = await db.getAllAsync<{
    task_id: string;
    title: string;
    description: string;
    created_at: string;
  }>(`SELECT task_id, title, description, created_at FROM archive_records ORDER BY created_at DESC`);
  return rows;
}

export async function setAppSetting(key: string, value: string) {
  const db = await getDb();
  await db.runAsync(
    `INSERT OR REPLACE INTO app_settings(key, value) VALUES(?, ?)`,
    key,
    value
  );
}

export async function getAppSetting(key: string) {
  const db = await getDb();
  const row = await db.getFirstAsync<{ value: string }>(
    `SELECT value FROM app_settings WHERE key = ?`,
    key
  );
  return row?.value ?? null;
}

export async function addStagedInput(input: {
  id: string;
  input_type: string;
  label: string;
  uri?: string | null;
  mime_type?: string | null;
  text_excerpt?: string | null;
  provenance: string;
  created_at: string;
}) {
  const db = await getDb();
  await db.runAsync(
    `INSERT OR REPLACE INTO staged_inputs(id, input_type, label, uri, mime_type, text_excerpt, provenance, created_at)
     VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
    input.id,
    input.input_type,
    input.label,
    input.uri ?? null,
    input.mime_type ?? null,
    input.text_excerpt ?? null,
    input.provenance,
    input.created_at
  );
}

export async function listStagedInputs() {
  const db = await getDb();
  return db.getAllAsync<{
    id: string;
    input_type: string;
    label: string;
    uri: string | null;
    mime_type: string | null;
    text_excerpt: string | null;
    provenance: string;
    created_at: string;
  }>(
    `SELECT id, input_type, label, uri, mime_type, text_excerpt, provenance, created_at
     FROM staged_inputs
     ORDER BY created_at DESC`
  );
}

export async function clearStagedInputs() {
  const db = await getDb();
  await db.runAsync(`DELETE FROM staged_inputs`);
}
