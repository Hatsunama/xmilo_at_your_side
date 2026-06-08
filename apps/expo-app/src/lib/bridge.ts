import type { CommandSubmitResponse, EventEnvelope } from "../types/contracts";
import { addArchiveRecord, initArchiveDb } from "./archiveDb";
import { SIDECAR_BASE_URL } from "./config";
import type { SettingsReportSubmitPayload } from "./reportBundle";
import {
  captureTaskAttemptIdentityFromSidecarPayload,
  clearCurrentTaskAttemptIdentity,
  resolveLocalhostBearerToken
} from "./xmiloRuntimeHost";

export class SidecarRequestError extends Error {
  path: string;
  status: number;
  errorClass?: string;
  errorCode?: string;

  constructor(message: string, path: string, status: number, fields?: { errorClass?: string; errorCode?: string }) {
    super(message);
    this.name = "SidecarRequestError";
    this.path = path;
    this.status = status;
    this.errorClass = fields?.errorClass;
    this.errorCode = fields?.errorCode;
  }
}

async function request<T = any>(path: string, init?: RequestInit): Promise<T> {
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
    throw new SidecarRequestError(toUserFacingLocalProviderError(String((error as Error)?.message ?? error)), path, 0);
  }

  const text = await response.text();
  let data: any = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = { raw: text };
  }

  if (!response.ok) {
    const rawMessage = String(data?.error_code === "ACTIVE_TASK_RUNNING" ? data.error_code : data?.error ?? data?.reason ?? response.statusText);
    throw new SidecarRequestError(toUserFacingLocalProviderError(rawMessage), path, response.status, {
      errorClass: safeResponseString(data?.error_class),
      errorCode: safeResponseString(data?.error_code)
    });
  }
  captureTaskAttemptIdentityFromSidecarPayload(data, `sidecar:${path}`);
  if (path === "/state" && isObjectRecord(data) && !data.active_task) {
    clearCurrentTaskAttemptIdentity("sidecar:/state:no_active_task");
  }
  if (path === "/task/current" && isObjectRecord(data) && !data.task) {
    clearCurrentTaskAttemptIdentity("sidecar:/task/current:no_task");
  }
  if (path === "/task/interrupt") {
    clearCurrentTaskAttemptIdentity("sidecar:/task/interrupt");
  }
  return data as T;
}

export function toUserFacingLocalProviderError(rawMessage: string) {
  switch (rawMessage) {
    case "entitlement_lost":
      return "Local API key access is not ready yet. Choose an access path in setup, then try again.";
    case "missing_local_provider_key":
      return "Missing local provider key. Choose Use my own API key in setup.";
    case "local_provider_auth_failed":
      return "Local provider authentication failed. Check the saved key and selected provider.";
    case "local_provider_rate_limited":
      return "Local provider rate limited this request. Wait a moment or choose another provider/model.";
    case "local_provider_unreachable":
      return "Local provider is unreachable from this phone. Check the provider connection and network.";
    case "local_provider_request_failed":
      return "Local provider request failed. Check provider status, model, and local configuration.";
    case "local_provider_unavailable":
      return "Local provider is unavailable right now. Check the provider service or local Ollama server.";
    case "missing_key":
      return "Provider API key is required before saving.";
    case "local_provider_base_url_required":
      return "Provider connection settings are incomplete.";
    case "local_provider_connection_target_required":
      return "Ollama Local must be running on this phone or on a computer your phone can reach before xMilo can use it.";
    case "local_provider_manual_url_invalid":
      return "Check the Ollama Local server address and try again.";
    case "local_provider_disallowed_url_scheme":
      return "Local provider URL scheme is not allowed for this provider.";
    case "local_provider_custom_base_url_not_allowed":
      return "Custom provider server address is not allowed for this provider.";
    case "local_provider_model_required":
      return "Local provider model is required before saving.";
    case "Network request failed":
    case "Failed to fetch":
      return "Local runtime route is still warming up. Wait a moment and try again.";
    case "ACTIVE_TASK_RUNNING":
    case "active task already running":
      return "Milo already has a task running. Wait for it to finish, or stop it before retrying.";
    default:
      return sanitizeErrorMessage(rawMessage);
  }
}

function sanitizeErrorMessage(message: string) {
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
  if (!/^[a-zA-Z0-9_.:-]{1,80}$/.test(clean)) return undefined;
  return clean;
}

export async function getHealth() {
  return request("/health");
}

export type SidecarReadyStatus = {
  ok: boolean;
  llm_mode?: string;
  byok_provider?: string;
  subscription_entitled?: boolean;
  bring_your_own_key_active?: boolean;
  phase9_api_key_access?: boolean;
  first_task_eligible?: boolean;
  relay_llm_turn_allowed?: boolean;
  local_llm_turn_allowed?: boolean;
};

export async function getReady(): Promise<SidecarReadyStatus> {
  return request("/ready");
}

export type LocalProviderOption = {
  id: string;
  label: string;
  default_model: string;
  default_base_url: string;
  key_required: boolean;
  custom_base_url_allowed: boolean;
  base_url_required: boolean;
  allowed_schemes: string[];
  connection_kind: string;
  base_url_entry_mode: string;
  advanced_manual_base_url_allowed: boolean;
  requires_connection_target: boolean;
  requires_reachability_probe: boolean;
  setup_guidance_code?: string;
};

export type LocalProviderResolveRequest = {
  provider: string;
  base_url?: string;
  model?: string;
  key_file_ready?: boolean;
  has_api_key?: boolean;
};

export type LocalProviderResolvedConfig = {
  provider: string;
  model: string;
  base_url: string;
  key_required: boolean;
  base_url_required: boolean;
  local_turn_allowed: boolean;
  readiness_reason?: string;
  spec?: LocalProviderOption;
  connection_kind: string;
  base_url_entry_mode: string;
  advanced_manual_base_url_allowed: boolean;
  requires_connection_target: boolean;
  requires_reachability_probe: boolean;
  setup_guidance_code?: string;
};

export async function getLocalProviderOptions(): Promise<LocalProviderOption[]> {
  const response = await request<{ providers?: LocalProviderOption[] }>("/local-provider/options");
  return response.providers ?? [];
}

export async function resolveLocalProviderConfig(input: LocalProviderResolveRequest): Promise<LocalProviderResolvedConfig> {
  const response = await request<{ resolved: LocalProviderResolvedConfig }>("/local-provider/resolve", {
    method: "POST",
    body: JSON.stringify(input)
  });
  return response.resolved;
}

export async function getState() {
  return request("/state");
}

export async function getTaskCurrent() {
  return request("/task/current");
}

export async function startTask(prompt: string) {
  return request("/task/start", {
    method: "POST",
    body: JSON.stringify({ prompt })
  });
}

export async function submitCommand(prompt: string): Promise<CommandSubmitResponse> {
  return request<CommandSubmitResponse>("/command/submit", {
    method: "POST",
    body: JSON.stringify({ prompt })
  });
}

export async function taskInterrupt() {
  return request("/task/interrupt", {
    method: "POST",
    body: JSON.stringify({})
  });
}

export async function getStorageStats() {
  return request("/storage/stats");
}

export type StagedContextPayload = {
  content: string;
  source: string;
  provenance: string;
  label?: string;
  mime_type?: string | null;
  created_at?: string;
};

export async function setContext(payload: StagedContextPayload) {
  return request("/context/set", {
    method: "POST",
    body: JSON.stringify(payload)
  });
}

export async function clearContext() {
  return request("/context/clear", {
    method: "POST",
    body: JSON.stringify({})
  });
}

export async function resetTier(tier: string) {
  return request("/reset", {
    method: "POST",
    body: JSON.stringify({ tier })
  });
}

export async function authRegister(email: string, deviceUserID: string) {
  return request("/auth/register", {
    method: "POST",
    body: JSON.stringify({ email, device_user_id: deviceUserID })
  });
}

export async function authCheck(): Promise<{
  device_user_id?: string;
  entitled: boolean;
  expires_at: string;
  access_mode?: string;
  llm_mode?: string;
  byok_provider?: string;
  subscription_entitled?: boolean;
  bring_your_own_key_active?: boolean;
  phase9_api_key_access?: boolean;
  first_task_eligible?: boolean;
  relay_llm_turn_allowed?: boolean;
  local_llm_turn_allowed?: boolean;
  access_code_only?: boolean;
  trial_allowed?: boolean;
  subscription_allowed?: boolean;
  access_code_grant_days?: number;
  verified_email?: string;
  email_verified?: boolean;
  two_factor_enabled?: boolean;
  two_factor_ok?: boolean;
  website_handoff_ready?: boolean;
}> {
  return request("/auth/check", {
    method: "POST",
    body: JSON.stringify({})
  });
}

export async function authRedeemInvite(code: string, deviceUserID: string) {
  return request("/auth/invite", {
    method: "POST",
    body: JSON.stringify({ code, device_user_id: deviceUserID })
  });
}

export async function authLogout() {
  return request("/auth/logout", {
    method: "POST",
    body: JSON.stringify({})
  });
}

export async function authDeleteAccount() {
  return request("/auth/account/delete", {
    method: "POST",
    body: JSON.stringify({})
  });
}

export async function getTwoFactorStatus() {
  return request("/auth/2fa/status");
}

export async function beginTwoFactorSetup() {
  return request("/auth/2fa/setup/begin", {
    method: "POST",
    body: JSON.stringify({})
  });
}

export async function confirmTwoFactorSetup(code: string) {
  return request("/auth/2fa/setup/confirm", {
    method: "POST",
    body: JSON.stringify({ code })
  });
}

export async function verifyTwoFactor(code: string, recoveryCode = "") {
  return request("/auth/2fa/verify", {
    method: "POST",
    body: JSON.stringify({ code, recovery_code: recoveryCode })
  });
}

export async function disableTwoFactor(code: string, recoveryCode = "") {
  return request("/auth/2fa/disable", {
    method: "POST",
    body: JSON.stringify({ code, recovery_code: recoveryCode })
  });
}

export async function regenerateTwoFactorRecoveryCodes() {
  return request("/auth/2fa/recovery/regenerate", {
    method: "POST",
    body: JSON.stringify({})
  });
}

export async function createWebsiteHandoff() {
  return request("/auth/website/handoff/create", {
    method: "POST",
    body: JSON.stringify({})
  });
}

export async function submitAIReport(payload: {
  task_id?: string;
  event_type: string;
  report_reason: string;
  report_note?: string;
  output_text: string;
  prompt_text?: string;
  model_name?: string;
}) {
  return request("/report/ai", {
    method: "POST",
    body: JSON.stringify(payload)
  });
}

export type SettingsReportProof = {
  accepted?: boolean;
  report_id?: string;
  status?: string;
  received_at?: string;
  client_report_id?: string;
  bundle_hash?: string;
  duplicate?: boolean;
};

export async function submitSettingsReport(payload: SettingsReportSubmitPayload): Promise<SettingsReportProof> {
  return request<SettingsReportProof>("/report/settings", {
    method: "POST",
    body: JSON.stringify(payload)
  });
}

export function connectBridge(onEvent: (event: EventEnvelope) => void) {
  initArchiveDb().catch(() => null);

  let disposed = false;
  let socket: WebSocket | null = null;
  let attempt = 0;
  const backoff = [1000, 2000, 5000, 10000];

  const open = async () => {
    if (disposed) return;
    try {
      const bearer = await resolveLocalhostBearerToken();
      if (disposed) return;

      socket = new WebSocket(
        `${SIDECAR_BASE_URL.replace("http", "ws")}/ws?token=${encodeURIComponent(bearer)}`
      );

      socket.onopen = () => {
        attempt = 0;
      };

      socket.onmessage = async (event) => {
        const parsed = JSON.parse(event.data) as EventEnvelope;
        const tagged: EventEnvelope = { ...parsed, source: "sidecar_runtime" };
        if (isTaskTerminalEvent(tagged.type)) {
          clearCurrentTaskAttemptIdentity(`event:${tagged.type}`);
        } else {
          captureTaskAttemptIdentityFromSidecarPayload(tagged, `event:${tagged.type}`);
        }
        onEvent(tagged);

        if (tagged.type === "archive.record_created") {
          await addArchiveRecord({
            task_id: tagged.payload.task_id,
            title: tagged.payload.title,
            description: tagged.payload.description,
            created_at: tagged.payload.created_at
          });
        }
      };

      socket.onclose = () => {
        if (disposed) return;
        const delay = backoff[Math.min(attempt, backoff.length - 1)];
        attempt += 1;
        setTimeout(open, delay);
      };

      socket.onerror = () => {
        socket?.close();
      };
    } catch {
      if (disposed) return;
      const delay = backoff[Math.min(attempt, backoff.length - 1)];
      attempt += 1;
      setTimeout(open, delay);
    }
  };

  void open();

  return () => {
    disposed = true;
    socket?.close();
  };
}

function isTaskTerminalEvent(type: string) {
  return type === "task.completed" ||
    type === "task.failed" ||
    type === "task.blocked" ||
    type === "task.stuck" ||
    type === "task.cancelled" ||
    type === "task.interrupted";
}

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object";
}
