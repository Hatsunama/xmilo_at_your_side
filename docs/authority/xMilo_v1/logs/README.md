# Logs

Runtime logs are written here by xMilo Sidecar and local maintenance scripts.

## Log rotation policy
- supervisor.log and recovery.log are rotated at 30 days or 10MB, whichever comes first
- nightly_consolidate.sh deletes log files older than 30 days via `find -mtime +30`
- Do not let logs grow unbounded — storage management warnings will fire at 250MB combined or 1GB free device storage

## Log files
- supervisor.log — heartbeat and recovery events from xMilo Sidecar's maintenance loop
- recovery.log — task recovery and retry events
- YYYY-MM-DD.md files in memory/ — daily session notes (not in this directory)
