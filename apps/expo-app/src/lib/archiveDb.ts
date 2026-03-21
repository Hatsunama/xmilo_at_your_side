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
