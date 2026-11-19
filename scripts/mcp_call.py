#!/usr/bin/env python3
"""Minimal MCP stdio client for the yhat-agent MCP server.

Used by scripts/smoke-central.sh to drive the agent without a GUI client.

Usage:
    python3 scripts/mcp_call.py <tool> [key=value ...]

Values are parsed as JSON when possible, otherwise treated as strings.
The tool result is printed as JSON on stdout; the exit code is non-zero when
the call failed.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
from typing import Any


def call(tool: str, arguments: dict[str, Any]) -> dict[str, Any]:
    """Run one tools/call against the MCP server and return the raw answer."""
    binary = os.environ.get("YHAT_AGENT_BIN", "yhat-agent")
    proc = subprocess.Popen(
        [binary, "mcp"],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
        bufsize=1,
    )

    def send(message: dict[str, Any]) -> None:
        assert proc.stdin is not None
        proc.stdin.write(json.dumps(message) + "\n")
        proc.stdin.flush()

    def recv() -> dict[str, Any]:
        assert proc.stdout is not None
        while True:
            line = proc.stdout.readline()
            if not line:
                raise SystemExit("MCP server closed the connection")
            line = line.strip()
            if not line:
                continue
            message = json.loads(line)
            if message.get("id") is not None:
                return message

    send(
        {
            "jsonrpc": "2.0",
            "id": 1,
            "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "yhat-smoke", "version": "1.0.0"},
            },
        }
    )
    recv()
    send({"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}})
    send(
        {
            "jsonrpc": "2.0",
            "id": 2,
            "method": "tools/call",
            "params": {"name": tool, "arguments": arguments},
        }
    )
    result = recv()
    assert proc.stdin is not None
    proc.stdin.close()
    proc.wait(timeout=10)
    return result


def main() -> int:
    if len(sys.argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2
    tool = sys.argv[1]
    arguments: dict[str, Any] = {}
    for raw in sys.argv[2:]:
        key, _, value = raw.partition("=")
        try:
            arguments[key] = json.loads(value)
        except json.JSONDecodeError:
            arguments[key] = value

    result = call(tool, arguments)
    if "error" in result:
        print(json.dumps(result, indent=2, ensure_ascii=False))
        return 1

    content = result.get("result", {}).get("content", [])
    for item in content:
        if item.get("type") == "text":
            try:
                print(json.dumps(json.loads(item["text"]), indent=2, ensure_ascii=False))
            except json.JSONDecodeError:
                print(item["text"])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
