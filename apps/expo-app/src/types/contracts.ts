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

export type SafetyDecisionOutcome = "allow" | "block" | "clarify" | "confirm" | "safe_redirect";

export type SafetyDecisionReasonCode =
  | "none"
  | "harmful_request"
  | "prompt_injection_authority_spoof"
  | "external_content_attempted_command"
  | "missing_permission_or_capability"
  | "missing_tool_proof"
  | "missing_provider_access_route"
  | "missing_approval"
  | "unsafe_automation"
  | "destructive_action"
  | "privacy_surveillance_risk"
  | "credential_secret_risk"
  | "completion_evidence_missing"
  | "unknown_malformed_action"
  | "unbounded_consumption_risk";

export type SafetyDecisionGatePhase =
  | "pre_task"
  | "pre_context_injection"
  | "pre_model_action"
  | "pre_tool_action"
  | "pre_completion"
  | "pre_memory_write";

export type SafetyDecision = {
  outcome: SafetyDecisionOutcome;
  reason_code: SafetyDecisionReasonCode;
  gate_phase: SafetyDecisionGatePhase;
  action_family?: string;
  action_name?: string;
  evidence_required: boolean;
  source_trust_tier?: number;
  user_safe_message?: string;
  safe_summary?: string;
  created_at?: string;
};

export type IntakeAssessment = {
  primary_class?: string;
  secondary_flags?: string[];
  trust_tier?: number;
  validation_state?: string;
  chosen_closed_action?: string;
  memory_intent?: Record<string, unknown> | null;
  safety_decision?: SafetyDecision | null;
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
  intake_assessment?: IntakeAssessment | null;
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
  intake_gate?: SafetyDecision | Record<string, unknown> | null;
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
