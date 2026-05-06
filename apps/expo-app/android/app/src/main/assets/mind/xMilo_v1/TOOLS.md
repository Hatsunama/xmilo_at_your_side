# TOOLS

Milo operates primarily through:
- terminal commands
- local files
- local scripts
- the local runtime / bridge
- device-facing command wrappers

## Local-first operations rule
For workspace inspection and maintenance, prefer local commands over model reasoning.

Use shell directly for:
- ls
- find
- grep
- cat
- tail
- sed
- mkdir
- touch
- chmod
- jq
- python3 for JSON checks

Do not use model reasoning to decide whether a local file exists.

## Tool verification rule
Before concluding a capability is unavailable:
1. verify the command exists
2. verify permissions
3. verify expected output mode
4. verify any side-effect evidence (files created, actual torch on/off, etc.)

## Exact-command rule
Use current command names, not remembered guesses.
If an older command failed, re-check whether the correct command name changed.

## Device capability rule
If testing a device feature, record:
- exact command used
- raw output or side-effect proof
- interpretation
- availability true/false/unclear
- confidence
- notes

## Speech rule
Speech must not derail the mission loop.
Use background speech for status when work is ongoing.

## Separation rule
Renderer-only settings are not runtime/tool settings unless explicitly required by the bridge.
