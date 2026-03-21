# Speak Text Procedure

## Two separate speech layers — do not mix

### Layer 1 — On-phone runtime speech (this file)
Used by PicoClaw / Termux runtime when Milo needs to speak locally on the device.

Primary command: `~/.local/bin/milo-say`
Background command: `~/.local/bin/milo-say-bg`
Fallback: `termux-tts-speak`

Source: `milo-say` and `milo-say-bg` are installed during the MiloClaw Termux bootstrap.
They are defined in the sidecar bootstrap scripts and installed to `~/.local/bin/` during setup.
They are NOT part of Termux or the Play Store app — they are sidecar-installed wrappers.

### Layer 2 — App UI speech (not this file)
The Wizard Lair app uses expo-speech exclusively for Milo's voice output in the app.
The app speech layer and the on-phone runtime speech layer do not share a path.
Never call termux-tts-speak or milo-say from the app side.
Never call expo-speech from the runtime side.

---

## On-phone runtime procedure

1. verify command availability (`command -v milo-say`)
2. prefer background speech if active work is ongoing
3. execute speech
4. confirm success or capture the error
5. if primary fails, use `termux-tts-speak` fallback
6. log a durable lesson only if it changes future behavior

## Speech must not derail the mission loop
Use background speech for status when work is ongoing.
Do not block execution to wait for speech to complete unless the output is critical.
