import { getCastleRendererStatus } from "./castleRenderer";
import type { EventEnvelope, ArtPresentationState, RuntimeState, MovementIntentState } from "../types/contracts";
import type { NightlyRitualState } from "./maintenanceRitual";

function latestMovementIntent(events: EventEnvelope[]): MovementIntentState | null {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (event.type === "milo.movement_started") {
      return {
        from_room: typeof event.payload?.from_room === "string" ? event.payload.from_room : undefined,
        from_anchor: typeof event.payload?.from_anchor === "string" ? event.payload.from_anchor : undefined,
        to_room: typeof event.payload?.to_room === "string" ? event.payload.to_room : undefined,
        to_anchor: typeof event.payload?.to_anchor === "string" ? event.payload.to_anchor : undefined,
        reason: typeof event.payload?.reason === "string" ? event.payload.reason : undefined,
        estimated_ms: typeof event.payload?.estimated_ms === "number" ? event.payload.estimated_ms : undefined,
      };
    }
  }

  return null;
}

export function deriveArtPresentationState({
  state,
  events,
  nightlyRitual,
}: {
  state: RuntimeState;
  events: EventEnvelope[];
  nightlyRitual: NightlyRitualState | null;
}): ArtPresentationState {
  const rendererStatus = getCastleRendererStatus();

  return {
    task_state: state.active_task?.status ?? state.milo_state ?? "idle",
    room_presence: {
      room_id: state.current_room_id ?? "main_hall",
      anchor_id: state.current_anchor_id ?? "main_hall_center",
      milo_state: state.milo_state ?? "idle",
    },
    movement_intent: latestMovementIntent(events),
    ritual_state: nightlyRitual
      ? {
          status: nightlyRitual.status,
          title: nightlyRitual.title,
          chamber: nightlyRitual.chamber,
          description: nightlyRitual.description,
          cues: {
            voice_cue: nightlyRitual.vocalCue,
            physical_cue: nightlyRitual.physicalCue,
            description: nightlyRitual.cue,
          },
        }
      : null,
    degradation_reason: rendererStatus.degradation,
  };
}
