import type { CommandSubmitResponse, EventEnvelope } from "../types/contracts";
import { addArchiveRecord, initArchiveDb } from "./archiveDb";
import { SIDECAR_BASE_URL } from "./config";
import { resolveLocalhostBearerToken } from "./xmiloRuntimeHost";

async function request<T = any>(path: string, init?: RequestInit): Promise<T> {
  const bearer = await resolveLocalhostBearerToken();

  const response = await fetch(`${SIDECAR_BASE_URL}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${bearer}`,
      ...(init?.headers ?? {})
    }
  });

  const text = await response.text();
  let data: any = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = { raw: text };
  }

  if (!response.ok) {
    const rawMessage = String(data?.error_code === "ACTIVE_TASK_RUNNING" ? data.error_code : data?.error ?? data?.reason ?? response.statusText);
    throw new Error(toUserFacingLocalProviderError(rawMessage));
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
      return "Local provider is unreachable from this phone. Check the provider base URL and network.";
    case "local_provider_request_failed":
      return "Local provider request failed. Check provider status, model, and local configuration.";
    case "local_provider_unavailable":
      return "Local provider is unavailable right now. Check the provider service or local Ollama server.";
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

// ─── Core sidecar calls ───────────────────────────────────────────────────────

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

export async function setContext(content: string) {
  return request("/context/set", {
    method: "POST",
    body: JSON.stringify({ content })
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

// ─── Auth / setup flows ───────────────────────────────────────────────────────

/**
 * Sends an email verification request to the relay via sidecar.
 * The relay emails a verification link to the user.
 */
export async function authRegister(email: string, deviceUserID: string) {
  return request("/auth/register", {
    method: "POST",
    body: JSON.stringify({ email, device_user_id: deviceUserID })
  });
}

/**
 * Refreshes the current access contract exposed by the sidecar.
 * Hosted relay entitlement remains separate from Phase 9 local BYOK eligibility;
 * first-task UX should read first_task_eligible/local_llm_turn_allowed.
 */
export async function authCheck(): Promise<{
  device_user_id?: string;
  entitled: boolean;
  expires_at: string;
  access_mode?: string;
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

/**
 * Redeems a single-use access code for the currently configured launch period.
 */
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

// ─── WebSocket bridge ─────────────────────────────────────────────────────────

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
        onEvent(parsed);

        if (parsed.type === "archive.record_created") {
          await addArchiveRecord({
            task_id: parsed.payload.task_id,
            title: parsed.payload.title,
            description: parsed.payload.description,
            created_at: parsed.payload.created_at
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
