import * as DocumentPicker from "expo-document-picker";
import * as FileSystem from "expo-file-system";
import * as ImagePicker from "expo-image-picker";

import { addStagedInput } from "./archiveDb";

export type StagedInputRecord = {
  id: string;
  input_type: string;
  label: string;
  uri: string | null;
  mime_type: string | null;
  text_excerpt: string | null;
  provenance: string;
  created_at: string;
};

const TEXT_MIME_PREFIXES = ["text/"];
const TEXT_EXTENSIONS = [".txt", ".md", ".json", ".js", ".ts", ".tsx", ".jsx", ".csv", ".yaml", ".yml", ".log"];
const MAX_TEXT_BYTES = 256 * 1024;
const MAX_EXCERPT_CHARS = 1500;

function makeId(prefix: string) {
  return `${prefix}_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
}

function looksTextLike(name: string, mimeType: string | null | undefined) {
  if (mimeType && TEXT_MIME_PREFIXES.some((prefix) => mimeType.startsWith(prefix))) {
    return true;
  }

  const lower = name.toLowerCase();
  return TEXT_EXTENSIONS.some((ext) => lower.endsWith(ext));
}

async function maybeReadExcerpt(uri: string, name: string, mimeType: string | null | undefined, size?: number | null) {
  if (!looksTextLike(name, mimeType) || (size ?? 0) > MAX_TEXT_BYTES) {
    return null;
  }

  try {
    const content = await FileSystem.readAsStringAsync(uri);
    const trimmed = content.trim();
    if (!trimmed) return null;
    return trimmed.slice(0, MAX_EXCERPT_CHARS);
  } catch {
    return null;
  }
}

export async function stageDocumentPick() {
  const result = await DocumentPicker.getDocumentAsync({
    multiple: false,
    copyToCacheDirectory: true,
    type: "*/*"
  });

  if (result.canceled || !result.assets?.length) {
    return null;
  }

  const asset = result.assets[0];
  const createdAt = new Date().toISOString();
  const record: StagedInputRecord = {
    id: makeId("file"),
    input_type: "file",
    label: asset.name,
    uri: asset.uri ?? null,
    mime_type: asset.mimeType ?? null,
    text_excerpt: await maybeReadExcerpt(asset.uri, asset.name, asset.mimeType, asset.size),
    provenance: "document_picker",
    created_at: createdAt
  };

  await addStagedInput(record);
  return record;
}

export async function stageImagePick() {
  const permission = await ImagePicker.requestMediaLibraryPermissionsAsync();
  if (!permission.granted) {
    throw new Error("Photo library permission is required to attach images.");
  }

  const result = await ImagePicker.launchImageLibraryAsync({
    mediaTypes: ["images"],
    allowsMultipleSelection: false,
    quality: 1
  });

  if (result.canceled || !result.assets?.length) {
    return null;
  }

  const asset = result.assets[0];
  const createdAt = new Date().toISOString();
  const record: StagedInputRecord = {
    id: makeId("image"),
    input_type: "image",
    label: asset.fileName ?? `Image ${createdAt}`,
    uri: asset.uri ?? null,
    mime_type: asset.mimeType ?? "image/*",
    text_excerpt: null,
    provenance: "image_picker",
    created_at: createdAt
  };

  await addStagedInput(record);
  return record;
}

export function formatStagedInputsForContext(inputs: StagedInputRecord[]) {
  if (!inputs.length) {
    return "";
  }

  const lines = [
    "Staged local inputs for this task:",
    ...inputs.map((input, index) => {
      const excerpt = input.text_excerpt ? ` | excerpt: ${input.text_excerpt.replace(/\s+/g, " ").slice(0, 240)}` : "";
      return `${index + 1}. [${input.input_type}] ${input.label}${input.mime_type ? ` | mime: ${input.mime_type}` : ""}${excerpt}`;
    })
  ];

  return lines.join("\n");
}
