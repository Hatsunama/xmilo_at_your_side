import { getAppSetting, initArchiveDb, setAppSetting } from "./archiveDb";

const SETUP_COMPLETED_ONCE_KEY = "setup_completed_once";

export async function hasSetupCompletedOnce() {
  await initArchiveDb();
  const value = await getAppSetting(SETUP_COMPLETED_ONCE_KEY);
  return value === "true";
}

export async function markSetupCompletedOnce() {
  await initArchiveDb();
  await setAppSetting(SETUP_COMPLETED_ONCE_KEY, "true");
}

