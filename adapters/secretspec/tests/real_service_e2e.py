#!/usr/bin/env python3
"""Exercise the real SecretSpec IPC endpoint against a running NEXUS-Q service."""
import json
import os
import subprocess
import time
from pathlib import Path

binary = Path(os.environ["ADAPTER_BIN"]).resolve()
store = Path(os.environ["NEXUSQ_STORE_DIR"]).resolve()
secret = 'SecretSpec/NEXUS-Q round trip: "quoted"; slash=\\; newline=first\nsecond; unicode=🔐'
address = {"kind": "convention", "project": "payments", "profile": "integration", "key": "DATABASE_URL"}


def request(process, request_id, method, params):
    frame = {
        "jsonrpc": "2.0",
        "id": request_id,
        "method": method,
        "_meta": {"deadline_unix_ms": int(time.time() * 1000) + 30000},
        "params": params,
    }
    process.stdin.write(json.dumps(frame, ensure_ascii=False, separators=(",", ":")) + "\n")
    process.stdin.flush()
    line = process.stdout.readline()
    if not line:
        raise RuntimeError("provider exited before replying to " + method)
    response = json.loads(line)
    if response.get("id") != request_id:
        raise RuntimeError("provider returned an unexpected JSON-RPC response")
    if "error" in response:
        raise RuntimeError(method + " failed with JSON-RPC error " + str(response["error"].get("code")))
    return response["result"]


def launch(context_id):
    env = os.environ.copy()
    env["NEXUSQ_CONTEXT_ID"] = context_id
    process = subprocess.Popen(
        [str(binary), "provider"],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
        encoding="utf-8",
        bufsize=1,
        env=env,
    )
    init = request(process, 1, "rpc.initialize", {
        "protocol": "secretspec.provider",
        "versions": [1],
        "client": {"name": "nexusq-adapter-e2e", "version": "1"},
        "limits": {"max_frame_bytes": 1048576, "max_in_flight": 8},
        "client_methods": [],
        "application": {
            "scheme": "nexusq",
            "uri": "nexusq://default",
            "context": {
                "project": "payments",
                "profile": "integration",
                "base_dir": None,
                "reason": "real-service integration test",
                "requested_authorization_duration_ms": None,
            },
        },
    })
    if init["protocol"] != "secretspec.provider" or init["version"] != 1:
        raise RuntimeError("provider did not negotiate secretspec.provider/1")
    return process


def stop(process):
    try:
        process.stdin.close()
        process.terminate()
        process.wait(timeout=5)
    except Exception:
        process.kill()
        process.wait(timeout=5)


def main():
    store.mkdir(mode=0o700, parents=True, exist_ok=True)
    process = launch("github-actions-secretspec-real-service-v1")
    try:
        resolved = request(process, 2, "provider.resolve_address", {"address": address})
        if not resolved.get("coordinates", {}).get("item", "").startswith("nexusq-v1:"):
            raise RuntimeError("convention address was not mapped to an opaque native address")
        result = request(process, 3, "provider.set", {"address": address, "value": secret})
        if result.get("stored") is not True:
            raise RuntimeError("provider did not confirm the write")
        exists = request(process, 4, "provider.exists", {"address": address})
        if exists.get("exists") is not True:
            raise RuntimeError("stored secret was not found")
        got = request(process, 5, "provider.get", {"address": address})
        if got.get("status") != "found" or got.get("value") != secret:
            raise RuntimeError("NEXUS-Q round trip did not reproduce the original secret")

        files = list(store.glob("*.json"))
        if len(files) != 1:
            raise RuntimeError("expected exactly one ciphertext record")
        serialized = files[0].read_text(encoding="utf-8")
        if secret in serialized or "DATABASE_URL" in serialized or "payments" in serialized:
            raise RuntimeError("local record contains plaintext or a cleartext address")

        wrong_context = launch("wrong-secretspec-context")
        try:
            try:
                request(wrong_context, 2, "provider.get", {"address": address})
                raise RuntimeError("wrong context unexpectedly decrypted a secret")
            except RuntimeError as error:
                if "JSON-RPC error" not in str(error):
                    raise
        finally:
            stop(wrong_context)

        deleted = request(process, 6, "provider.delete", {"address": address})
        if deleted.get("deleted") is not True:
            raise RuntimeError("provider did not delete the stored envelope")
        missing = request(process, 7, "provider.get", {"address": address})
        if missing.get("status") != "missing":
            raise RuntimeError("deleted secret was not reported missing")
    finally:
        stop(process)


if __name__ == "__main__":
    main()
