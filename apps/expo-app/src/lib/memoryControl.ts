import { SIDECAR_BASE_URL } from "./config";
import { resolveLocalhostBearerToken } from "./xmiloRuntimeHost";

export type MemoryAction =
  | "view"
  | "view_provenance"
  | "suppress"
  | "restore_suppression"
  | "delete_user_remove"
  | "correct_supersede"
  | "mark_stale"
  | "reject_candidate";

export type MemoryFilter = "all" | "important" | "suppressed" | "review";

export type VisibleMemoryProjection = {
  memory_id: string;
  memory_class: string;
  status: string;
  category?: string;
  title: string;
  summary: string;
  content_excerpt?: string;
  source_type: string;
  source_id?: string;
  freshness_state?: string;
  confidence?: number;
  contradiction_state?: string;
  quarantine_status?: string;
  suppression_status?: string;
  retrieval_eligible?: boolean;
  retrieval_reason?: string;
  rollback_available?: boolean;
  user_visible?: boolean;
  warnings?: string[];
  allowed_actions: string[];
};

export type MemoryEvidenceProjection = {
  evidence_id: string;
  source_type: string;
  source_id: string;
  source_ref: string;
  evidence_kind: string;
  trust_tier: number;
  authority_rank: string;
  timestamp: string;
  content_hash: string;
  redaction_status: string;
  display_allowed: boolean;
  promotion_allowed: boolean;
};

export type MemoryCandidateProjection = {
  candidate_id: string;
  candidate_type: string;
  status: string;
  title: string;
  summary: string;
  source_type: string;
  source_id?: string;
  trust_tier?: number;
  authority_rank?: string;
  freshness_state?: string;
  confidence?: number;
  contradiction_state?: string;
  quarantine_status?: string;
  suppression_status?: string;
  warnings?: string[];
  allowed_actions: string[];
};

export type MemoryActionRequest = {
  reason?: string;
  confirmation?: boolean;
  title?: string;
  summary?: string;
  content?: string;
  content_excerpt?: string;
};

export type MemoryActionResponse = {
  ok: boolean;
  action: string;
  status: string;
  audit_id?: string;
  memory?: VisibleMemoryProjection;
  corrected_memory?: VisibleMemoryProjection;
  candidate?: MemoryCandidateProjection;
  allowed_actions?: string[];
  warnings?: string[];
  confirmation_required?: boolean;
  message?: string;
};

export function hasMemoryAction(item: { allowed_actions?: string[] }, action: MemoryAction) {
  return item.allowed_actions?.includes(action) === true;
}

export function isProtectedMemoryProjection(memory: VisibleMemoryProjection) {
  return (
    memory.memory_class === "canon_memory" ||
    memory.memory_class === "runtime_observation" ||
    memory.source_type === "canon" ||
    memory.source_type === "main_hub" ||
    memory.source_type === "verified_runtime" ||
    hasMemoryWarning(memory, "protected")
  );
}

export function isSuppressedMemoryProjection(memory: VisibleMemoryProjection) {
  const status = memory.status.toLowerCase();
  return (
    status === "suppressed" ||
    status === "deleted_by_user" ||
    status === "stale" ||
    memory.suppression_status === "suppressed" ||
    memory.retrieval_eligible === false
  );
}

export function isImportantMemoryProjection(memory: VisibleMemoryProjection) {
  return isProtectedMemoryProjection(memory) || (memory.confidence ?? 0) >= 0.85 || hasMemoryWarning(memory, "important");
}

export function memoryMatchesFilter(memory: VisibleMemoryProjection, filter: MemoryFilter) {
  switch (filter) {
    case "important":
      return isImportantMemoryProjection(memory) && !isSuppressedMemoryProjection(memory);
    case "suppressed":
      return isSuppressedMemoryProjection(memory);
    case "review":
      return false;
    default:
      return !isSuppressedMemoryProjection(memory);
  }
}

export function memoryMatchesSearch(memory: VisibleMemoryProjection, query: string) {
  const clean = query.trim().toLowerCase();
  if (!clean) return true;
  return [
    memory.title,
    memory.summary,
    memory.content_excerpt,
    memory.memory_class,
    memory.category,
    memory.source_type,
    memory.status
  ]
    .filter(Boolean)
    .some((value) => String(value).toLowerCase().includes(clean));
}

export function candidateMatchesSearch(candidate: MemoryCandidateProjection, query: string) {
  const clean = query.trim().toLowerCase();
  if (!clean) return true;
  return [candidate.title, candidate.summary, candidate.candidate_type, candidate.status, candidate.source_type]
    .filter(Boolean)
    .some((value) => String(value).toLowerCase().includes(clean));
}

export function visibleListActions(memory: VisibleMemoryProjection) {
  const actions: MemoryAction[] = [];
  if (hasMemoryAction(memory, "correct_supersede")) actions.push("correct_supersede");
  if (hasMemoryAction(memory, "suppress")) actions.push("suppress");
  if (hasMemoryAction(memory, "restore_suppression")) actions.push("restore_suppression");
  actions.push("view");
  return actions;
}

export function memoryMutationConfirmed(response: MemoryActionResponse) {
  return response.ok === true && typeof response.audit_id === "string" && response.audit_id.trim().length > 0;
}

function hasMemoryWarning(memory: VisibleMemoryProjection, fragment: string) {
  return memory.warnings?.some((warning) => warning.toLowerCase().includes(fragment)) === true;
}

type Requester = <T>(path: string, init?: RequestInit) => Promise<T>;

export class MemoryControlRequestError extends Error {
  path: string;
  status: number;
  errorCode?: string;

  constructor(message: string, path: string, status: number, errorCode?: string) {
    super(message);
    this.name = "MemoryControlRequestError";
    this.path = path;
    this.status = status;
    this.errorCode = errorCode;
  }
}

async function memoryRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const bearer = await resolveLocalhostBearerToken();
  let response: Response;
  try {
    response = await fetch(`${SIDECAR_BASE_URL}${path}`, {
      ...init,
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${bearer}`,
        ...(init?.headers ?? {})
      }
    });
  } catch (error) {
    throw new MemoryControlRequestError(toUserFacingMemoryError(String((error as Error)?.message ?? error)), path, 0);
  }

  const text = await response.text();
  let data: any = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = { raw: text };
  }

  if (!response.ok) {
    throw new MemoryControlRequestError(memoryErrorMessage(data), path, response.status, safeResponseString(data?.error_code));
  }

  return data as T;
}

function memoryErrorMessage(data: any) {
  const code = safeResponseString(data?.error_code) || safeResponseString(data?.error);
  switch (code) {
    case "sidecar_auth_missing_or_invalid":
      return "Memory controls are unavailable until the local sidecar is connected.";
    case "memory_forbidden_action":
    case "memory_canon_memory_cannot_modify":
    case "memory_runtime_truth_cannot_modify":
    case "memory_protected_truth_cannot_modify":
      return "That memory can't be changed here.";
    case "memory_confirmation_required":
      return "Please confirm this memory action before continuing.";
    case "memory_not_found":
      return "That memory item is no longer available.";
    case "memory_unsafe_secret_blocked":
      return "That update looks like it may contain a secret, so xMilo did not save it.";
    case "memory_approved_summary_correction_deferred":
      return "Approved summary correction is not available in this version.";
    case "memory_candidate_approval_deferred":
      return "Candidate approval is not available in this version.";
    case "memory_rollback_deferred":
      return "Rollback is not available in this version.";
    default:
      return toUserFacingMemoryError(String(code || data?.reason || data?.error || "Memory controls are unavailable."));
  }
}

function toUserFacingMemoryError(rawMessage: string) {
  switch (rawMessage) {
    case "Network request failed":
    case "Failed to fetch":
      return "Memory controls are unavailable until the local sidecar is connected.";
    default:
      return sanitizeMemoryError(rawMessage);
  }
}

function sanitizeMemoryError(message: string) {
  return message
    .replace(/eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}/g, "<redacted>")
    .replace(/authorization\s*[:=]\s*bearer\s+[^\s,"'}]+/gi, "authorization: bearer <redacted>")
    .replace(/bearer\s+[A-Za-z0-9._-]{16,}/gi, "bearer <redacted>")
    .replace(/[A-Za-z0-9_-]{48,}/g, "<redacted>")
    .slice(0, 240);
}

function safeResponseString(value: unknown) {
  if (typeof value !== "string") return undefined;
  const clean = value.trim();
  if (!/^[a-zA-Z0-9_.:-]{1,100}$/.test(clean)) return undefined;
  return clean;
}

function actionPath(memoryId: string, action: "provenance" | "suppress" | "restore" | "delete" | "correct" | "mark-stale") {
  return `/memory/${encodeURIComponent(memoryId)}/${action}`;
}

function candidateRejectPath(candidateId: string) {
  return `/memory/candidates/${encodeURIComponent(candidateId)}/reject`;
}

export function createMemoryControlClient(requester: Requester = memoryRequest) {
  const postAction = <T>(path: string, payload: MemoryActionRequest = {}) =>
    requester<T>(path, { method: "POST", body: JSON.stringify(payload) });

  return {
    async listMemory() {
      const response = await requester<{ ok?: boolean; memories?: VisibleMemoryProjection[] }>("/memory");
      return response.memories ?? [];
    },
    async getMemoryDetail(memoryId: string) {
      const response = await requester<{ ok?: boolean; memory?: VisibleMemoryProjection }>(`/memory/${encodeURIComponent(memoryId)}`);
      if (!response.memory) throw new Error("Memory detail was empty.");
      return response.memory;
    },
    async getMemoryProvenance(memoryId: string) {
      return requester<{ ok?: boolean; memory_id?: string; evidence?: MemoryEvidenceProjection[]; audit_id?: string }>(
        actionPath(memoryId, "provenance")
      );
    },
    suppressMemory(memoryId: string, payload: MemoryActionRequest = {}) {
      return postAction<MemoryActionResponse>(actionPath(memoryId, "suppress"), payload);
    },
    restoreMemory(memoryId: string, payload: MemoryActionRequest = {}) {
      return postAction<MemoryActionResponse>(actionPath(memoryId, "restore"), payload);
    },
    deleteMemory(memoryId: string, payload: MemoryActionRequest = {}) {
      return postAction<MemoryActionResponse>(actionPath(memoryId, "delete"), { ...payload, confirmation: payload.confirmation === true });
    },
    correctMemory(memoryId: string, payload: MemoryActionRequest) {
      return postAction<MemoryActionResponse>(actionPath(memoryId, "correct"), payload);
    },
    markMemoryStale(memoryId: string, payload: MemoryActionRequest = {}) {
      return postAction<MemoryActionResponse>(actionPath(memoryId, "mark-stale"), payload);
    },
    async listMemoryCandidates() {
      const response = await requester<{ ok?: boolean; candidates?: MemoryCandidateProjection[] }>("/memory/candidates");
      return response.candidates ?? [];
    },
    rejectMemoryCandidate(candidateId: string, payload: MemoryActionRequest = {}) {
      return postAction<MemoryActionResponse>(candidateRejectPath(candidateId), payload);
    }
  };
}

export const memoryControl = createMemoryControlClient();
