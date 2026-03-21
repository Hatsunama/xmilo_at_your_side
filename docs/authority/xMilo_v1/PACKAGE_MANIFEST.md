# xMilo_v1 — Package Manifest

## Included authority sets

### 1. Patched Mind v4 canonical pack
Contains the corrected policy and knowledge layer:
- root policy files
- tacit memory
- knowledge memory
- maintenance utilities / retired script evidence
- migrated task/state evidence

### 2. PicoClaw Go responsibility checklist
Contains the engine-side implementation checklist for:
- localhost bridge and WebSocket transport
- SQLite schema and migrations
- task lifecycle
- auth/JWT lifecycle
- room routing and movement
- archive/trophy/report/reset/storage
- legacy import
- WebSocket reliability
- nightly maintenance

### 3. Wizard Lair blocker answers v16
Contains the locked app/product/platform decisions for:
- Android back button behavior
- relay/network-loss retry handling
- first-launch intro and guided first task
- notification tap routing
- chat markdown/copy behavior
- input limits
- task timeout
- trial warning / subscriptions / restore purchases
- OTA scope
- rating prompt
- accessibility baseline
- fixed dark theme

## Historical evidence preserved

The following are intentionally preserved as evidence and should not be read as live runtime authority:
- `scripts/*.retired.sh`
- `tasks/*.migrated.json`
- `state/*.migrated.*`
- `tasks/opportunities.retired.json`

## Packaging rule

This zip is the single integrated pack.
Do not split it back into separate “Mind” and “delta” zip files for implementation work.
