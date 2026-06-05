# Reviewed Anchors

## Definition

A reviewed anchor is a human-reviewed baseline result or evidence packet used for regression comparison.

## When Required

Reviewed anchors are required for high-risk fixtures, live-model fixtures, phone/device proof, policy-sensitive behavior, and any release-blocking regression gate.

## False Confidence Control

Anchors prevent false confidence by separating "a runner produced output" from "a qualified reviewer accepted this as a stable comparison point." A passing dry run is not a reviewed anchor by itself.

## Boundary

Reviewed anchors are not production truth. They do not prove that app, sidecar, relay, memory, or phone behavior is correct in production. They support regression comparison only.

## Use In Phase 19-Memory

Phase 19-Memory should create anchors only after the fixture schema, result schema, dry-run gate, and cache behavior are stable.

