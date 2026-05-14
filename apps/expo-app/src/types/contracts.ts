export type EventEnvelope = {
  type: string;
  timestamp: string;
  source?: "sidecar_runtime" | "android_bridge_observation" | "ui_local";
  truth_scope?: "sidecar_health" | "sidecar_ready" | "native_runtime_host" | "task_route_surface" | "capability_state" | "ui_submit";
  payload: Record<string, any>;
};

export type RuntimeRecoveryResultState =
  | "restart_verified"
  | "restart_failed"
  | "restart_rate_limited"
  | "restart_needs_operator";

export type RuntimeRecoverySource = "sidecar_runtime" | "android_bridge_observation" | "ui_local";

export type RuntimeRecoveryTruthScope =
  | "sidecar_health"
  | "sidecar_ready"
  | "native_runtime_host"
  | "task_route_surface"
  | "ui_submit";

export type RuntimeRecoveryOutcome = {
  result: RuntimeRecoveryResultState;
  source: RuntimeRecoverySource;
  truth_scope: RuntimeRecoveryTruthScope[];
  verified: boolean;
  checked_at: string;
  note: string;
  blocking_reason?: string;
  attempts_used: number;
  next_allowed_at?: string;
};

export type ProviderDiagnosticPayload = {
  task_id?: string;
  attempt_id?: string;
  error_code?: string;
  code?: string;
  error_category?: string;
  category?: string;
  provider?: string;
  base_url_host?: string;
  endpoint_path?: string;
  http_status?: number;
  network_class?: string;
  provider_error_class?: string;
};

export type TaskSnapshot = {
  task_id: string;
  attempt_id: string;
  prompt: string;
  intent: string;
  room_id: string;
  anchor_id: string;
  status: string;
  started_at: string;
  updated_at: string;
  retry_count: number;
  max_retries: number;
  failure_type?: string;
  stuck_reason?: string;
  slot?: string;
};

export type RuntimeState = {
  milo_state: string;
  current_room_id: string;
  current_anchor_id: string;
  last_meaningful_user_action_at: string;
  active_task?: TaskSnapshot | null;
  queued_task?: TaskSnapshot | null;
  capability_state?: Record<string, any> | null;
  runtime_id: string;
  pending_approval?: boolean | null;
  resume_checkpoint?: string | null;
};

export type CommandSubmitResponse = {
  handled?: boolean;
  kind?: "task" | "movement" | string;
  task_id?: string;
  attempt_id?: string;
  immediate_state?: TaskSnapshot | null;
  intake_gate?: Record<string, unknown> | null;
  plan?: Record<string, unknown> | null;
};

export type CueDescriptor = {
  voice_cue?: string;
  physical_cue?: string;
  description?: string;
};

export type RitualArtState = {
  status: string;
  title: string;
  chamber: string;
  description: string;
  cues: CueDescriptor;
};

export type RoomPresenceState = {
  room_id: string;
  anchor_id: string;
  milo_state: string;
};

export type MovementIntentState = {
  from_room?: string;
  from_anchor?: string;
  to_room?: string;
  to_anchor?: string;
  reason?: string;
  estimated_ms?: number;
};

export type ArtDegradationState = {
  reason?: string;
  surface?: string;
  message?: string;
};

export type ArtPresentationState = {
  task_state: string;
  room_presence: RoomPresenceState;
  movement_intent?: MovementIntentState | null;
  ritual_state?: RitualArtState | null;
  degradation_reason?: ArtDegradationState | null;
};
