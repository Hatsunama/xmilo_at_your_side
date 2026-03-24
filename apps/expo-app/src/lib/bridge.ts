import type { EventEnvelope } from "../types/contracts";
import { addArchiveRecord, initArchiveDb } from "./archiveDb";
import { LOCALHOST_TOKEN, SIDECAR_BASE_URL } from "./config";

const headers = () => ({
  "Content-Type": "application/json",
  Authorization: `Bearer ${LOCALHOST_TOKEN}`
});

async function request(path: string, init?: RequestInit) {
  if (!LOCALHOST_TOKEN) {
    throw new Error("Missing EXPO_PUBLIC_LOCALHOST_TOKEN");
  }

  const response = await fetch(`${SIDECAR_BASE_URL}${path}`, {
    ...init,
    headers: {
      ...headers(),
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
    throw new Error(data?.error ?? data?.reason ?? response.statusText);
  }
  return data;
}

// ─── Core sidecar calls ───────────────────────────────────────────────────────

export async function getHealth() {
  return request("/health");
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

export async function submitCommand(prompt: string) {
  return request("/command/submit", {
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
 * Forces an immediate relay JWT refresh and returns the current entitled status.
 * Call after user taps "I verified my email" — returns entitled=true within seconds
 * of the relay verifying the token.
 */
export async function authCheck(): Promise<{
  entitled: boolean;
  expires_at: string;
  access_mode?: string;
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
  if (!LOCALHOST_TOKEN) {
    return () => undefined;
  }

  initArchiveDb().catch(() => null);

  let disposed = false;
  let socket: WebSocket | null = null;
  let attempt = 0;
  const backoff = [1000, 2000, 5000, 10000];

  const open = () => {
    if (disposed) return;
    socket = new WebSocket(
      `${SIDECAR_BASE_URL.replace("http", "ws")}/ws?token=${encodeURIComponent(LOCALHOST_TOKEN)}`
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
  };

  open();

  return () => {
    disposed = true;
    socket?.close();
  };
}
