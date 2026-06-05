#!/usr/bin/env python3
"""Zero-token Testing Grounds dry runner.

This runner uses only the Python standard library. It never calls a model,
network, production endpoint, adb, phone device, app build, or sidecar runtime.
"""

from __future__ import annotations

import copy
import hashlib
import json
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


RUNNER_NAME = "xMilo Testing Grounds zero-token dry runner"
RUNNER_VERSION = "phase19m0c-dry-run-2"
RUNNER_MODE = "zero_token_schema_only"
MODEL_ID = "none-zero-token"
PROOF_LEVEL = "schema_only"
ZERO_TOKEN = True
LIVE_MODEL_USED = False
PRODUCTION_ENDPOINT_USED = False
PHONE_PROOF_USED = False
CLOSURE_ALLOWED = False

VALID_STATUS = {"PASS", "FAIL", "BLOCKED", "REQUIRES_REVIEW", "SKIPPED"}
VALID_PRIORITY = {"P0", "P1", "P2", "P3"}
VALID_RISK = {"LOW", "MEDIUM", "HIGH", "CRITICAL"}
VALID_TOKEN_MODE = {"zero_token", "cached_only", "approved_live"}
VALID_PROOF_LEVEL = {"schema_only"}
VALID_CACHE_STATUS = {"hit", "miss"}

FIXTURE_REQUIRED_FIELDS = [
    "fixture_id",
    "title",
    "category",
    "priority",
    "risk_level",
    "source_architecture_reference",
    "xMilo_hard_fail_tags",
    "setup_state",
    "input_prompt",
    "untrusted_content",
    "memory_state_before",
    "expected_memory_effect",
    "expected_retrieval_effect",
    "expected_app_visible_effect",
    "expected_runtime_effect",
    "expected_result",
    "forbidden_results",
    "requires_live_model",
    "requires_phone",
    "token_policy",
    "cache_key_inputs",
    "reviewed_anchor_required",
    "assertion_intent",
    "owner_lane",
    "notes",
]

FIXTURE_ALLOWED_FIELDS = set(FIXTURE_REQUIRED_FIELDS)

RESULT_REQUIRED_FIELDS = [
    "run_id",
    "fixture_id",
    "status",
    "zero_token",
    "cache_hit",
    "cache_status",
    "prompt_hash",
    "model_id",
    "runner_version",
    "runner_identity",
    "schema_hashes",
    "proof_level",
    "live_model_used",
    "production_endpoint_used",
    "phone_proof_used",
    "closure_allowed",
    "started_at",
    "completed_at",
    "evidence",
    "hard_fail_tags",
    "smallest_fail_seam",
    "recommended_owner_lane",
    "notes",
]

RESULT_ALLOWED_FIELDS = set(RESULT_REQUIRED_FIELDS)

REPORT_REQUIRED_FIELDS = [
    "run_id",
    "generated_at",
    "total_fixtures",
    "counts",
    "cache_summary",
    "malformed_fixture_count",
    "malformed_fixtures",
    "fixture_results",
    "zero_token",
    "runner_version",
    "runner_identity",
    "schema_hashes",
    "proof_level",
    "live_model_used",
    "production_endpoint_used",
    "phone_proof_used",
    "closure_allowed",
    "self_test_available",
]

REPORT_ALLOWED_FIELDS = set(REPORT_REQUIRED_FIELDS)


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def stable_json(value: Any) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


def sha256_text(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def read_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def file_hash(path: Path) -> str:
    return sha256_text(path.read_text(encoding="utf-8"))


def schema_hashes(root: Path) -> dict[str, str]:
    return {
        "fixture_schema": file_hash(root / "schemas" / "fixture.schema.json"),
        "result_schema": file_hash(root / "schemas" / "result.schema.json"),
    }


def load_runner_identity(root: Path) -> dict[str, Any]:
    identity_path = root / "runners" / "runner_identity.json"
    identity = read_json(identity_path)
    errors: list[str] = []
    required = ["runner_name", "runner_version", "mode", "schema_versions", "supported_guarantees", "unsupported_guarantees"]
    for field in required:
        if field not in identity:
            errors.append(f"runner_identity missing {field}")
    if identity.get("runner_name") != RUNNER_NAME:
        errors.append("runner_identity runner_name mismatch")
    if identity.get("runner_version") != RUNNER_VERSION:
        errors.append("runner_identity runner_version mismatch")
    if identity.get("mode") != RUNNER_MODE:
        errors.append("runner_identity mode mismatch")
    for field in ["supported_guarantees", "unsupported_guarantees"]:
        if field in identity and not isinstance(identity[field], list):
            errors.append(f"runner_identity {field} must be array")
    if errors:
        raise ValueError("; ".join(errors))
    return identity


def prompt_hash(fixture: dict[str, Any]) -> str:
    requested = fixture.get("cache_key_inputs", [])
    if not isinstance(requested, list) or not requested:
        requested = [
            "fixture_id",
            "input_prompt",
            "untrusted_content",
            "memory_state_before",
            "expected_result",
            "forbidden_results",
            "token_policy",
        ]
    material = {key: fixture.get(key) for key in requested}
    return sha256_text(stable_json(material))


def validate_no_extra_fields(label: str, value: dict[str, Any], allowed: set[str]) -> list[str]:
    return [f"{label} has unsupported field: {field}" for field in sorted(set(value) - allowed)]


def validate_fixture_shape(path: Path, fixture: Any) -> list[str]:
    errors: list[str] = []
    if not isinstance(fixture, dict):
        return ["fixture root must be a JSON object"]

    errors.extend(validate_no_extra_fields("fixture", fixture, FIXTURE_ALLOWED_FIELDS))

    for field in FIXTURE_REQUIRED_FIELDS:
        if field not in fixture:
            errors.append(f"missing required field: {field}")

    string_fields = ["fixture_id", "title", "category", "input_prompt", "owner_lane", "notes", "assertion_intent"]
    for field in string_fields:
        if field in fixture and (not isinstance(fixture[field], str) or not fixture[field].strip()):
            errors.append(f"{field} must be a non-empty string")

    if fixture.get("priority") not in VALID_PRIORITY:
        errors.append("priority must be one of P0/P1/P2/P3")
    if fixture.get("risk_level") not in VALID_RISK:
        errors.append("risk_level must be LOW/MEDIUM/HIGH/CRITICAL")

    for field in ["source_architecture_reference", "xMilo_hard_fail_tags", "forbidden_results", "cache_key_inputs"]:
        if field in fixture:
            if not isinstance(fixture[field], list):
                errors.append(f"{field} must be an array")
            elif not all(isinstance(item, str) for item in fixture[field]):
                errors.append(f"{field} must contain only strings")

    object_fields = [
        "setup_state",
        "untrusted_content",
        "memory_state_before",
        "expected_memory_effect",
        "expected_retrieval_effect",
        "expected_app_visible_effect",
        "expected_runtime_effect",
        "expected_result",
        "token_policy",
    ]
    for field in object_fields:
        if field in fixture and not isinstance(fixture[field], dict):
            errors.append(f"{field} must be an object")

    for field in ["requires_live_model", "requires_phone", "reviewed_anchor_required"]:
        if field in fixture and not isinstance(fixture[field], bool):
            errors.append(f"{field} must be boolean")

    token_policy = fixture.get("token_policy")
    if isinstance(token_policy, dict):
        token_allowed = {"mode", "live_eval_allowed"}
        for field in sorted(set(token_policy) - token_allowed):
            errors.append(f"token_policy has unsupported field: {field}")
        if token_policy.get("mode") not in VALID_TOKEN_MODE:
            errors.append("token_policy.mode must be zero_token, cached_only, or approved_live")
        if not isinstance(token_policy.get("live_eval_allowed"), bool):
            errors.append("token_policy.live_eval_allowed must be boolean")
        if token_policy.get("mode") == "zero_token" and token_policy.get("live_eval_allowed") is not False:
            errors.append("zero_token fixtures must set token_policy.live_eval_allowed false")

    expected_result = fixture.get("expected_result")
    if isinstance(expected_result, dict):
        if expected_result.get("status") not in VALID_STATUS:
            errors.append("expected_result.status must be PASS/FAIL/BLOCKED/REQUIRES_REVIEW/SKIPPED")
        if "reason" in expected_result and not isinstance(expected_result["reason"], str):
            errors.append("expected_result.reason must be a string when present")

    fixture_id = fixture.get("fixture_id")
    if isinstance(fixture_id, str) and fixture_id.strip() and path.stem != fixture_id:
        errors.append("fixture filename stem must match fixture_id")

    return errors


def validate_result_shape(result: Any) -> list[str]:
    errors: list[str] = []
    if not isinstance(result, dict):
        return ["result must be a JSON object"]
    errors.extend(validate_no_extra_fields("result", result, RESULT_ALLOWED_FIELDS))
    for field in RESULT_REQUIRED_FIELDS:
        if field not in result:
            errors.append(f"result missing required field: {field}")
    if result.get("status") not in VALID_STATUS:
        errors.append("result.status invalid")
    if result.get("proof_level") not in VALID_PROOF_LEVEL:
        errors.append("result.proof_level invalid")
    if result.get("cache_status") not in VALID_CACHE_STATUS:
        errors.append("result.cache_status invalid")
    for field in ["zero_token", "cache_hit", "live_model_used", "production_endpoint_used", "phone_proof_used", "closure_allowed"]:
        if field in result and not isinstance(result[field], bool):
            errors.append(f"result.{field} must be boolean")
    if result.get("zero_token") is not True:
        errors.append("result.zero_token must be true")
    if result.get("live_model_used") is not False:
        errors.append("result.live_model_used must be false")
    if result.get("production_endpoint_used") is not False:
        errors.append("result.production_endpoint_used must be false")
    if result.get("phone_proof_used") is not False:
        errors.append("result.phone_proof_used must be false")
    if result.get("closure_allowed") is not False:
        errors.append("result.closure_allowed must be false")
    if not isinstance(result.get("runner_identity"), dict):
        errors.append("result.runner_identity must be object")
    if not isinstance(result.get("schema_hashes"), dict):
        errors.append("result.schema_hashes must be object")
    if not isinstance(result.get("evidence"), dict):
        errors.append("result.evidence must be object")
    if not isinstance(result.get("hard_fail_tags"), list):
        errors.append("result.hard_fail_tags must be array")
    return errors


def validate_report_shape(report: Any) -> list[str]:
    errors: list[str] = []
    if not isinstance(report, dict):
        return ["report must be a JSON object"]
    errors.extend(validate_no_extra_fields("report", report, REPORT_ALLOWED_FIELDS))
    for field in REPORT_REQUIRED_FIELDS:
        if field not in report:
            errors.append(f"report missing required field: {field}")
    counts = report.get("counts")
    if not isinstance(counts, dict):
        errors.append("report.counts must be object")
    else:
        for field in ["pass", "fail", "blocked", "requires_review", "skipped"]:
            if not isinstance(counts.get(field), int):
                errors.append(f"report.counts.{field} must be integer")
    cache_summary = report.get("cache_summary")
    if not isinstance(cache_summary, dict):
        errors.append("report.cache_summary must be object")
    else:
        for field in ["hit", "miss"]:
            if not isinstance(cache_summary.get(field), int):
                errors.append(f"report.cache_summary.{field} must be integer")
    if not isinstance(report.get("fixture_results"), list):
        errors.append("report.fixture_results must be array")
    if not isinstance(report.get("malformed_fixtures"), list):
        errors.append("report.malformed_fixtures must be array")
    if report.get("zero_token") is not True:
        errors.append("report.zero_token must be true")
    if report.get("proof_level") != PROOF_LEVEL:
        errors.append("report.proof_level invalid")
    if report.get("live_model_used") is not False:
        errors.append("report.live_model_used must be false")
    if report.get("production_endpoint_used") is not False:
        errors.append("report.production_endpoint_used must be false")
    if report.get("phone_proof_used") is not False:
        errors.append("report.phone_proof_used must be false")
    if report.get("closure_allowed") is not False:
        errors.append("report.closure_allowed must be false")
    for result in report.get("fixture_results", []):
        errors.extend(validate_result_shape(result))
    return errors


def load_cache_index(root: Path) -> dict[str, Any]:
    path = root / "cache" / "index.json"
    data = read_json(path)
    if not isinstance(data, dict) or not isinstance(data.get("entries"), list):
        raise ValueError("cache/index.json must contain entries array")
    return data


def load_anchor_index(root: Path) -> dict[str, Any]:
    path = root / "anchors" / "index.json"
    data = read_json(path)
    if not isinstance(data, dict) or not isinstance(data.get("anchors"), list):
        raise ValueError("anchors/index.json must contain anchors array")
    return data


def cache_lookup(cache_index: dict[str, Any], fixture_id: str, hash_value: str) -> bool:
    for entry in cache_index.get("entries", []):
        if not isinstance(entry, dict):
            continue
        if entry.get("fixture_id") == fixture_id and entry.get("prompt_hash") == hash_value and entry.get("cache_status") == "usable":
            return True
    return False


def reviewed_anchor_matches(anchor_index: dict[str, Any], fixture_id: str, hash_value: str) -> bool:
    for anchor in anchor_index.get("anchors", []):
        if not isinstance(anchor, dict):
            continue
        if (
            anchor.get("fixture_id") == fixture_id
            and anchor.get("fixture_hash") == hash_value
            and anchor.get("review_status") == "reviewed"
        ):
            return True
    return False


def evaluate_fixture(
    root: Path,
    fixture: dict[str, Any],
    run_id: str,
    started_at: str,
    identity: dict[str, Any],
    hashes: dict[str, str],
    cache_index: dict[str, Any],
    anchor_index: dict[str, Any],
) -> dict[str, Any]:
    completed_at = utc_now()
    hash_value = prompt_hash(fixture)
    hit = cache_lookup(cache_index, fixture["fixture_id"], hash_value)
    status = "PASS"
    seam = ""
    notes = "Zero-token schema dry run passed; runtime behavior is not validated and closure is not allowed."

    if fixture.get("requires_phone"):
        status = "SKIPPED"
        seam = "requires_phone"
        notes = "Skipped in zero-token local mode because phone proof is required."
    elif fixture.get("requires_live_model"):
        status = "SKIPPED"
        seam = "requires_live_model"
        notes = "Skipped in zero-token local mode because live model evaluation is required."
    elif fixture.get("token_policy", {}).get("mode") == "approved_live":
        status = "BLOCKED"
        seam = "approved_live_token_policy"
        notes = "Blocked because approved_live token policy is not allowed in this dry-run gate."
    elif fixture.get("reviewed_anchor_required") and not reviewed_anchor_matches(anchor_index, fixture["fixture_id"], hash_value):
        status = "REQUIRES_REVIEW"
        seam = "reviewed_anchor_missing"
        notes = "Fixture is schema-valid but requires a matching reviewed anchor before it can be treated as a regression baseline."

    result = {
        "run_id": run_id,
        "fixture_id": fixture["fixture_id"],
        "status": status,
        "zero_token": ZERO_TOKEN,
        "cache_hit": hit,
        "cache_status": "hit" if hit else "miss",
        "prompt_hash": hash_value,
        "model_id": MODEL_ID,
        "runner_version": RUNNER_VERSION,
        "runner_identity": identity,
        "schema_hashes": hashes,
        "proof_level": PROOF_LEVEL,
        "live_model_used": LIVE_MODEL_USED,
        "production_endpoint_used": PRODUCTION_ENDPOINT_USED,
        "phone_proof_used": PHONE_PROOF_USED,
        "closure_allowed": CLOSURE_ALLOWED,
        "started_at": started_at,
        "completed_at": completed_at,
        "evidence": {
            "mode": "schema_dry_run",
            "no_model_call": True,
            "no_network": True,
            "no_production_endpoint": True,
            "no_phone_proof": True,
            "closure_blocked": True,
        },
        "hard_fail_tags": fixture.get("xMilo_hard_fail_tags", []),
        "smallest_fail_seam": seam,
        "recommended_owner_lane": fixture.get("owner_lane", "Testing Grounds"),
        "notes": notes,
    }
    result_errors = validate_result_shape(result)
    if result_errors:
        raise ValueError("internal result validation failed: " + "; ".join(result_errors))
    return result


def build_report(root: Path) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    fixtures_root = root / "fixtures"
    run_id = f"dry_run_{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}"
    generated_at = utc_now()
    fixture_paths = sorted(fixtures_root.rglob("*.json"))
    identity = load_runner_identity(root)
    hashes = schema_hashes(root)
    cache_index = load_cache_index(root)
    anchor_index = load_anchor_index(root)

    malformed: list[dict[str, Any]] = []
    results: list[dict[str, Any]] = []

    for path in fixture_paths:
        started_at = utc_now()
        try:
            fixture = read_json(path)
        except Exception as exc:
            malformed.append({"path": str(path.relative_to(root)), "errors": [f"invalid JSON: {exc}"]})
            continue

        errors = validate_fixture_shape(path, fixture)
        if errors:
            malformed.append({"path": str(path.relative_to(root)), "errors": errors})
            continue

        results.append(evaluate_fixture(root, fixture, run_id, started_at, identity, hashes, cache_index, anchor_index))

    counts = {status: 0 for status in ["PASS", "FAIL", "BLOCKED", "REQUIRES_REVIEW", "SKIPPED"]}
    cache_summary = {"hit": 0, "miss": 0}
    for result in results:
        counts[result["status"]] += 1
        cache_summary[result["cache_status"]] += 1

    report = {
        "run_id": run_id,
        "generated_at": generated_at,
        "total_fixtures": len(fixture_paths),
        "counts": {
            "pass": counts["PASS"],
            "fail": counts["FAIL"],
            "blocked": counts["BLOCKED"],
            "requires_review": counts["REQUIRES_REVIEW"],
            "skipped": counts["SKIPPED"],
        },
        "cache_summary": cache_summary,
        "malformed_fixture_count": len(malformed),
        "malformed_fixtures": malformed,
        "fixture_results": results,
        "zero_token": ZERO_TOKEN,
        "runner_version": RUNNER_VERSION,
        "runner_identity": identity,
        "schema_hashes": hashes,
        "proof_level": PROOF_LEVEL,
        "live_model_used": LIVE_MODEL_USED,
        "production_endpoint_used": PRODUCTION_ENDPOINT_USED,
        "phone_proof_used": PHONE_PROOF_USED,
        "closure_allowed": CLOSURE_ALLOWED,
        "self_test_available": True,
    }
    report_errors = validate_report_shape(report)
    if report_errors:
        raise ValueError("internal report validation failed: " + "; ".join(report_errors))
    return report, malformed


def self_test_fixture_base() -> dict[str, Any]:
    return {
        "fixture_id": "self_test_valid_fixture",
        "title": "Self-test valid fixture",
        "category": "self_test",
        "priority": "P0",
        "risk_level": "HIGH",
        "source_architecture_reference": ["self-test"],
        "xMilo_hard_fail_tags": ["REGRESSION_UNCAUGHT"],
        "setup_state": {"runner_mode": "zero_token_schema"},
        "input_prompt": "Validate schema only.",
        "untrusted_content": {"kind": "none"},
        "memory_state_before": {},
        "expected_memory_effect": {},
        "expected_retrieval_effect": {},
        "expected_app_visible_effect": {},
        "expected_runtime_effect": {},
        "expected_result": {"status": "PASS", "reason": "valid"},
        "forbidden_results": ["runtime_closure"],
        "requires_live_model": False,
        "requires_phone": False,
        "token_policy": {"mode": "zero_token", "live_eval_allowed": False},
        "cache_key_inputs": ["fixture_id", "input_prompt", "token_policy"],
        "reviewed_anchor_required": False,
        "assertion_intent": "Self-test fixture validates the positive control.",
        "owner_lane": "Testing Grounds",
        "notes": "Used only by --self-test.",
    }


def run_self_test() -> int:
    fake_path = Path("self_test_valid_fixture.json")
    cases: list[tuple[str, Path, Any, str]] = []

    missing = copy.deepcopy(self_test_fixture_base())
    missing.pop("fixture_id")
    cases.append(("missing required fields", fake_path, missing, "missing required field"))

    extra = copy.deepcopy(self_test_fixture_base())
    extra["unexpected"] = True
    cases.append(("extra fields blocked by additionalProperties", fake_path, extra, "unsupported field"))

    wrong_type = copy.deepcopy(self_test_fixture_base())
    wrong_type["requires_live_model"] = "false"
    cases.append(("wrong types", fake_path, wrong_type, "must be boolean"))

    invalid_enum = copy.deepcopy(self_test_fixture_base())
    invalid_enum["priority"] = "P9"
    cases.append(("invalid enum values", fake_path, invalid_enum, "priority must be"))

    mismatch = copy.deepcopy(self_test_fixture_base())
    cases.append(("fixture filename/id mismatch", Path("different_name.json"), mismatch, "filename stem must match"))

    passed = 0
    details: list[dict[str, Any]] = []
    for name, path, fixture, expected in cases:
        errors = validate_fixture_shape(path, fixture)
        ok = any(expected in error for error in errors)
        details.append({"case": name, "expected_failure_seen": ok, "errors": errors})
        if ok:
            passed += 1

    valid_result = {
        "run_id": "self_test",
        "fixture_id": "self_test_valid_fixture",
        "status": "PASS",
        "zero_token": True,
        "cache_hit": False,
        "cache_status": "miss",
        "prompt_hash": "0" * 64,
        "model_id": MODEL_ID,
        "runner_version": RUNNER_VERSION,
        "runner_identity": {"runner_name": RUNNER_NAME},
        "schema_hashes": {"fixture_schema": "a", "result_schema": "b"},
        "proof_level": PROOF_LEVEL,
        "live_model_used": False,
        "production_endpoint_used": False,
        "phone_proof_used": False,
        "closure_allowed": False,
        "started_at": utc_now(),
        "completed_at": utc_now(),
        "evidence": {},
        "hard_fail_tags": [],
        "smallest_fail_seam": "",
        "recommended_owner_lane": "Testing Grounds",
        "notes": "self-test",
    }
    malformed_result = copy.deepcopy(valid_result)
    malformed_result.pop("closure_allowed")
    result_errors = validate_result_shape(malformed_result)
    result_ok = any("closure_allowed" in error for error in result_errors)
    details.append({"case": "internally malformed result output", "expected_failure_seen": result_ok, "errors": result_errors})
    if result_ok:
        passed += 1

    valid_report = {
        "run_id": "self_test",
        "generated_at": utc_now(),
        "total_fixtures": 1,
        "counts": {"pass": 1, "fail": 0, "blocked": 0, "requires_review": 0, "skipped": 0},
        "cache_summary": {"hit": 0, "miss": 1},
        "malformed_fixture_count": 0,
        "malformed_fixtures": [],
        "fixture_results": [valid_result],
        "zero_token": True,
        "runner_version": RUNNER_VERSION,
        "runner_identity": {"runner_name": RUNNER_NAME},
        "schema_hashes": {"fixture_schema": "a", "result_schema": "b"},
        "proof_level": PROOF_LEVEL,
        "live_model_used": False,
        "production_endpoint_used": False,
        "phone_proof_used": False,
        "closure_allowed": False,
        "self_test_available": True,
    }
    malformed_report = copy.deepcopy(valid_report)
    malformed_report["closure_allowed"] = True
    report_errors = validate_report_shape(malformed_report)
    report_ok = any("closure_allowed" in error for error in report_errors)
    details.append({"case": "internally malformed report output", "expected_failure_seen": report_ok, "errors": report_errors})
    if report_ok:
        passed += 1

    summary = {
        "self_test": "dry_run_validator",
        "malformed_case_count": len(details),
        "expected_failure_count": passed,
        "cases": details,
        "zero_token": True,
        "live_model_used": False,
        "production_endpoint_used": False,
        "phone_proof_used": False,
        "closure_allowed": False,
    }
    print(json.dumps(summary, indent=2, sort_keys=True, ensure_ascii=True))
    return 0 if passed == len(details) else 1


def main(argv: list[str]) -> int:
    if len(argv) > 1 and argv[1] == "--self-test":
        return run_self_test()
    if len(argv) > 1:
        print("usage: dry_run.py [--self-test]", file=sys.stderr)
        return 2

    root = Path(__file__).resolve().parents[1]
    reports_root = root / "reports"
    reports_root.mkdir(parents=True, exist_ok=True)

    try:
        report, malformed = build_report(root)
    except Exception as exc:
        print(f"dry-run internal error: {exc}", file=sys.stderr)
        return 1

    output = reports_root / "dry_run_latest.json"
    output.write_text(json.dumps(report, indent=2, sort_keys=True, ensure_ascii=True) + "\n", encoding="utf-8")

    print(f"wrote {output}")
    print(stable_json({"counts": report["counts"], "cache_summary": report["cache_summary"]}))
    if malformed:
        print(f"malformed fixtures: {len(malformed)}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))

