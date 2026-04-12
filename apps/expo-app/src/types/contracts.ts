export type EventEnvelope = {
  type: string;
  timestamp: string;
  payload: Record<string, any>;
};

export type TaskSnapshot = {
  task_id: string;
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
  runtime_id: string;
  pending_approval?: boolean | null;
  resume_checkpoint?: string | null;
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
