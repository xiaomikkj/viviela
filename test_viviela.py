#!/usr/bin/env python3
"""Quick sanity check for the viviela refactor."""

import os
import re

REPO = "/run/csi/mount-root/nas/4079184d856ecc166ed19d4887083405/workspaces/default/viviela-push"


def check(path, expect_present, expect_absent):
    full = os.path.join(REPO, path)
    if not os.path.exists(full):
        return f"MISSING: {path}"
    with open(full, "r", encoding="utf-8") as f:
        content = f.read()
    missing = [s for s in expect_present if s not in content]
    bad = [s for s in expect_absent if s in content]
    if missing or bad:
        return f"FAIL: {path} missing={missing} bad={bad}"
    return f"OK: {path}"


def main():
    results = [
        check(
            "pkg/agent/memory_manager.go",
            ["BuildMemoryContextBlock", "SyncTurn", "QueuePrefetch", "Shutdown"],
            [],
        ),
        check(
            "pkg/agent/turn_context.go",
            ["TurnContext", "NewTurnContext", "WithRuntimeEvents", "WithTurnID", "WithAgentID"],
            [],
        ),
        check(
            "pkg/agent/curator.go",
            ["NewCurator", "Start()", "Stop()", "refreshSkillTreeMtime", "refreshMemoryMtime"],
            [],
        ),
        check(
            "pkg/agent/context.go",
            ["memoryManager", "BuildMemoryContextBlock", "memoryManager ...*MemoryManager"],
            [],
        ),
        check(
            "pkg/agent/instance.go",
            ["MemoryManager", "NewMemoryManager", "memoryManager"],
            [],
        ),
        check(
            "pkg/agent/turn_coord.go",
            ["MemoryManager", "SyncTurn", "QueuePrefetch"],
            [],
        ),
        check(
            "pkg/agent/agent.go",
            ["curator", "Start()", "Stop()"],
            [],
        ),
        check(
            "pkg/agent/agent_init.go",
            ["curator", "NewCurator"],
            [],
        ),
        check(
            "pkg/auth/oauth.go",
            ["os.Getenv(\"GOOGLE_OAUTH_CLIENT_ID\")", "os.Getenv(\"GOOGLE_OAUTH_CLIENT_SECRET\")"],
            ["MTA3MTAwNjA2MDU5MS", "R09DU1BYLUs1OEZXUjQ4NkxkTEoxbUxCOHNYQzR6NnFEQWY="],
        ),
    ]
    for r in results:
        print(r)


if __name__ == "__main__":
    main()
