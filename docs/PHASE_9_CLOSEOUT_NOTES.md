# Phase 9 Closeout Notes

## Hybrid 9B decision

Phase 9 keeps Main Hall stable as `/` and routes successful setup paths into `/lair`.
The first-run Lair overlay introduces Lair as the primary embodied xMilo space over the live scene.

Remaining debt: the canonical `/lair` true-home route change is deferred to Phase 10+ architecture work and must not be treated as silently complete.

## Phase 9C skill import audit

Current app surfaces do not include a skill import, skill search, or skill add entry point.

Decision: `NOT_APPLICABLE_NO_APP_IMPORT_SURFACE`.

If a skill import/search/add surface is introduced later, it must include the Phase 9C warning, explicit `Don't remind me again` persistence, and basic screening result bands before it can be considered closeout-safe.

## Strict setup permission gate

Phase 9 setup now treats media and location permission quality as active checks, not plain Android granted booleans.

- Photos and videos setup accepts only all photos/videos access. Limited selected-media access is rejected with repair copy: `Choose Allow all photos and videos, not limited access.`
- Location setup accepts only precise location. Approximate-only location is rejected with repair copy: `Choose Precise location.`
- Camera, microphone, and location permission rows still instruct the user to choose while-using access, but Android/RN/Expo inspection did not prove a reliable public app-accessible post-grant signal for distinguishing one-time grants from while-using grants. Capability state therefore reports `grant_scope: unknown` when Android only reports granted, and setup must not claim it proved while-using scope.
- Camera, location, and sensor capabilities remain permission/capability-state truth only for Phase 9; app-owned camera capture, full sensor readout, and image assessment stay Phase 10+ hardening.

## Provider/access switching parity

Settings now owns the post-setup access-mode switch path. The user can switch new tasks between hosted access and local BYOK providers through the app-owned native runtime host.

- BYOK saves write the selected provider/config through native storage, restart the runtime host, and verify `/ready` route truth before showing success.
- Hosted switching deactivates local BYOK routing without deleting hosted session state or saved BYOK config, then restarts and verifies the relay route.
- Blank BYOK key saves preserve the existing provider key; nonblank saves replace it for bad-key recovery.
- Provider switching is blocked while a task is active so the app does not silently move an in-flight task across access modes. New tasks after a verified switch use the current route.
