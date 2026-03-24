import type { EventEnvelope } from "../types/contracts";

export type NightlyRitualStatus = "deferred" | "started" | "completed";

export type NightlyRitualState = {
  status: NightlyRitualStatus;
  event: EventEnvelope;
  title: string;
  description: string;
  cue: string;
  chamber: string;
  visual: string;
  vocalCue: string;
  physicalCue: string;
};

function toNightlyRitualState(event: EventEnvelope): NightlyRitualState | null {
  switch (event.type) {
    case "maintenance.nightly_deferred":
      return {
        status: "deferred",
        event,
        title: "Nightly upkeep is waiting",
        description: "Milo reached the 2 AM ritual window but deferred upkeep until the active task is finished.",
        cue: "The observatory dims and holds its place until the current task is complete.",
        chamber: "Observatory",
        visual: "Moonlit instruments stay open, but the archive seal remains unbroken until the work is done.",
        vocalCue: "A low hush acknowledges the delay without interrupting the task.",
        physicalCue: "A restrained pulse waits in the background for the ritual window to reopen."
      };
    case "maintenance.nightly_started":
      return {
        status: "started",
        event,
        title: "Nightly upkeep has begun",
        description: "Milo is checking for app updates and sealing the day's archive without interrupting the user.",
        cue: "A soft chime and a brief shimmer mark the start of the ritual.",
        chamber: "Archive Observatory",
        visual: "Signals gather at the orb while the archive shelves begin their quiet sorting pass.",
        vocalCue: "A soft spoken cue announces that the nightly upkeep ritual is underway.",
        physicalCue: "A brief vibration marks the start of the maintenance rite."
      };
    case "maintenance.nightly_completed":
      return {
        status: "completed",
        event,
        title: "Nightly upkeep is complete",
        description: "The archive has been sealed and the update check has finished for this cycle.",
        cue: "A settling glow and a short confirmation cue mark the ritual's end.",
        chamber: "Sealed Archive",
        visual: "The shelves settle, the orb clears, and the castle returns to its resting glow.",
        vocalCue: "A short completion line confirms that the archive and update check are finished.",
        physicalCue: "A final confirming pulse marks the ritual's close."
      };
    default:
      return null;
  }
}

export function getLatestNightlyRitualState(events: EventEnvelope[]): NightlyRitualState | null {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const ritualState = toNightlyRitualState(events[index]);
    if (ritualState) {
      return ritualState;
    }
  }

  return null;
}
