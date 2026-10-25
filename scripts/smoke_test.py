#!/usr/bin/env python3
"""End-to-end smoke test: drive the MCP server over stdio like a real client.

Checks that the server initializes, advertises its tool set, and can complete a
round trip against a live New API instance using the one unauthenticated
endpoint (GET /api/status).

    NEWAPI_BASE_URL=http://127.0.0.1:3001 NEWAPI_ACCESS_TOKEN=x \
        python3 scripts/smoke_test.py
"""
import json
import os
import subprocess
import sys

BIN = os.environ.get("OPERATOR_BIN", "bin/newapi-operator")


def main() -> int:
    env = dict(os.environ)
    env.setdefault("NEWAPI_BASE_URL", "http://127.0.0.1:3001")
    env.setdefault("NEWAPI_ACCESS_TOKEN", "smoke-test-token")
    env.setdefault("LOG_LEVEL", "error")

    proc = subprocess.Popen(
        [BIN],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=env,
        text=True,
        bufsize=1,
    )

    def send(msg):
        proc.stdin.write(json.dumps(msg) + "\n")
        proc.stdin.flush()

    def recv():
        line = proc.stdout.readline()
        if not line:
            err = proc.stderr.read()
            raise RuntimeError(f"server closed stdout; stderr:\n{err}")
        return json.loads(line)

    failures = []

    send({
        "jsonrpc": "2.0", "id": 1, "method": "initialize",
        "params": {
            "protocolVersion": "2025-06-18",
            "capabilities": {},
            "clientInfo": {"name": "smoke", "version": "1"},
        },
    })
    init = recv()
    server = init.get("result", {}).get("serverInfo", {})
    print(f"initialize   -> {server.get('name')} {server.get('version')}")
    if server.get("name") != "newapi-operator":
        failures.append("unexpected server name")

    send({"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}})

    send({"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}})
    listing = recv()
    tools = listing.get("result", {}).get("tools", [])
    print(f"tools/list   -> {len(tools)} tools")
    if not tools:
        failures.append("no tools advertised")

    names = {t["name"] for t in tools}
    for required in ("channel_create", "token_create", "api_request", "api_endpoints"):
        if required not in names:
            failures.append(f"missing tool {required}")

    read_only = sum(1 for t in tools if t.get("annotations", {}).get("readOnlyHint"))
    print(f"             -> {read_only} read-only, {len(tools) - read_only} mutating")

    # The one call that works without an access token.
    send({
        "jsonrpc": "2.0", "id": 3, "method": "tools/call",
        "params": {"name": "user_session_info", "arguments": {}},
    })
    call = recv()
    if call.get("error"):
        failures.append(f"user_session_info transport error: {call['error']}")
    else:
        result = call.get("result", {})
        text = "".join(c.get("text", "") for c in result.get("content", []))
        if result.get("isError"):
            failures.append(f"user_session_info failed: {text[:400]}")
        else:
            try:
                payload = json.loads(text)
                version = payload.get("data", {}).get("version")
                print(f"user_session_info -> HTTP {payload.get('http_status')}, version {version}")
            except json.JSONDecodeError:
                failures.append(f"result was not JSON: {text[:200]}")

    # A mutating path must be refused when read-only is on. Skip if it is off.
    if os.environ.get("NEWAPI_READ_ONLY", "").lower() in ("1", "true"):
        send({
            "jsonrpc": "2.0", "id": 4, "method": "tools/call",
            "params": {"name": "channel_delete", "arguments": {"id": 1}},
        })
        ro = recv()
        text = "".join(c.get("text", "") for c in ro.get("result", {}).get("content", []))
        if "read-only" not in text.lower() and "NEWAPI_READ_ONLY" not in text:
            failures.append(f"read-only mode did not block channel_delete: {text[:200]}")
        else:
            print("read-only    -> mutation blocked")

    proc.stdin.close()
    proc.terminate()
    proc.wait(timeout=10)

    if failures:
        print("\nFAILED:")
        for f in failures:
            print(" -", f)
        return 1
    print("\nOK")
    return 0


if __name__ == "__main__":
    sys.exit(main())