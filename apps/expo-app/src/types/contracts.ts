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
};
