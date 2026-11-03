#!/usr/bin/env python3
"""fnox CLI integration tests against a deliberately non-cryptographic mock.

The mock wraps plaintext in a marker and Base64; it validates adapter plumbing,
not NEXUS-Q encryption, key custody, or cryptographic authenticity.
"""

from __future__ import annotations

import base64
import json
import os
import re
import shutil
import subprocess
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
PROVIDER = ROOT / "adapters" / "fnox" / "provider.lua"
TOKEN = "T" * 40
KEY_ID = "fnox-test-key"
MARKER = b"MOCK-NONCRYPTO\0"
MAX_RESPONSE = 2 * 1024 * 1024


class MockState:
    mode = "normal"
    redirect_captures = 0


class QuietThreadingHTTPServer(ThreadingHTTPServer):
    def handle_error(self, request, client_address):
        import sys
        error_type = sys.exc_info()[0]
        if error_type in (BrokenPipeError, ConnectionResetError):
            return
        super().handle_error(request, client_address)


class MockHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *_args):
        pass

    def send_json(self, status: int, value) -> None:
        body = json.dumps(value, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def authorized(self) -> bool:
        if self.headers.get("Authorization") != f"Bearer {TOKEN}":
            self.send_json(401, {"error": "unauthorized"})
            return False
        return True

    def do_GET(self):
        if not self.authorized():
            return
        if self.path == "/v1/keys":
            self.send_json(200, [{"key_id": KEY_ID, "algorithm": "mock", "purpose": "mock", "status": "active"}])
        else:
            self.send_json(404, {})

    def do_POST(self):
        if self.path == "/capture":
            MockState.redirect_captures += 1
            size = int(self.headers.get("Content-Length", "0"))
            self.rfile.read(size)
            self.send_json(200, {})
            return
        if not self.authorized():
            return
        size = int(self.headers.get("Content-Length", "0"))
        request_body = self.rfile.read(size)

        if self.path == "/v1/encrypt" and MockState.mode == "redirect":
            self.send_response(307)
            self.send_header("Location", f"http://127.0.0.1:{self.server.server_port}/capture")
            self.send_header("Content-Length", "0")
            self.end_headers()
            return

        if self.path == "/v1/decrypt" and MockState.mode == "locked":
            self.send_json(423, {"error": "locked"})
            return

        if self.path == "/v1/decrypt" and MockState.mode == "oversized":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Transfer-Encoding", "chunked")
            self.end_headers()
            remaining = MAX_RESPONSE + 256 * 1024
            chunk = b"x" * 16384
            try:
                while remaining > 0:
                    part = chunk[: min(len(chunk), remaining)]
                    self.wfile.write(f"{len(part):X}\r\n".encode() + part + b"\r\n")
                    self.wfile.flush()
                    remaining -= len(part)
                self.wfile.write(b"0\r\n\r\n")
            except (BrokenPipeError, ConnectionResetError):
                pass
            return

        try:
            obj = json.loads(request_body)
        except Exception:
            self.send_json(400, {})
            return

        if self.path == "/v1/encrypt":
            if obj.get("key_id") != KEY_ID or not isinstance(obj.get("plaintext"), str):
                self.send_json(400, {})
                return
            plaintext = obj["plaintext"]
            if MockState.mode == "wrong-version":
                payload = json.loads(plaintext)
                payload["version"] = "2"
                plaintext = json.dumps(payload, separators=(",", ":"))
            envelope = MARKER + plaintext.encode("utf-8")
            self.send_json(200, {"data": base64.b64encode(envelope).decode("ascii")})
            return

        if self.path == "/v1/decrypt":
            try:
                envelope = base64.b64decode(obj["envelope"], validate=True)
            except Exception:
                self.send_json(400, {})
                return
            if not envelope.startswith(MARKER):
                self.send_json(400, {})
                return
            self.send_json(200, {"data": base64.b64encode(envelope[len(MARKER):]).decode("ascii")})
            return

        self.send_json(404, {})


class FnoxCliTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        binary = os.environ.get("FNOX_BIN") or shutil.which("fnox")
        if not binary:
            raise RuntimeError("fnox CLI not found; set FNOX_BIN to fnox v1.47.0 or newer")
        cls.fnox = str(Path(binary).resolve())
        version = subprocess.run([cls.fnox, "--version"], check=True, capture_output=True, text=True).stdout
        match = re.search(r"fnox\s+(\d+)\.(\d+)\.(\d+)", version)
        if not match or tuple(map(int, match.groups())) < (1, 47, 0):
            raise RuntimeError(f"fnox 1.47.0 or newer required; found {version.strip()}")
        curl = subprocess.run(["curl", "--version"], check=True, capture_output=True, text=True).stdout
        match = re.search(r"^curl\s+(\d+)\.(\d+)\.(\d+)", curl)
        if not match or tuple(map(int, match.groups())) < (8, 4, 0):
            raise RuntimeError("curl 8.4.0 or newer is required")

        cls.server = QuietThreadingHTTPServer(("127.0.0.1", 0), MockHandler)
        cls.server_thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.server_thread.start()
        cls.url = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.server_thread.join(timeout=2)

    def setUp(self):
        MockState.mode = "normal"
        MockState.redirect_captures = 0
        self.temp = tempfile.TemporaryDirectory(prefix="nexusq-fnox-test-")
        self.project = Path(self.temp.name)
        self.config = self.project / "fnox.toml"
        self.config.write_text(
            "[providers.nexusq]\n"
            'type = "lua"\n'
            f'plugin = "{PROVIDER.as_posix()}"\n\n'
            "[secrets]\n",
            encoding="utf-8",
        )
        self.env = os.environ.copy()
        self.env.update({
            "NEXUSQ_URL": self.url,
            "NEXUSQ_TOKEN": TOKEN,
            "NEXUSQ_KEY_ID": KEY_ID,
            "NEXUSQ_CONTEXT_ID": "integration-test",
            "FNOX_NON_INTERACTIVE": "1",
        })

    def tearDown(self):
        self.temp.cleanup()

    def run_fnox(self, *args, input_text=None, env=None):
        return subprocess.run(
            [self.fnox, *args],
            cwd=self.project,
            env=env or self.env,
            input=input_text,
            capture_output=True,
            text=True,
            timeout=45,
        )

    def set_secret(self, value="test-secret"):
        result = self.run_fnox("set", "TEST_SECRET", "--provider", "nexusq", input_text=value)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_provider_test_and_set_get_round_trip_with_special_characters(self):
        result = self.run_fnox("provider", "test", "nexusq")
        self.assertEqual(result.returncode, 0, result.stderr)
        secret = 'quotes: "hello"; slash: \\; newline:\n; unicode: 🚀'
        self.set_secret(secret)
        stored = self.config.read_text(encoding="utf-8")
        self.assertIn("nexusq-fnox:v1:", stored)
        self.assertNotIn(secret, stored)
        resolved = self.run_fnox("get", "TEST_SECRET")
        self.assertEqual(resolved.returncode, 0, resolved.stderr)
        self.assertEqual(resolved.stdout, secret + "\n")

    def test_wrong_context_fails_closed(self):
        self.set_secret()
        wrong = self.env.copy()
        wrong["NEXUSQ_CONTEXT_ID"] = "other-context"
        result = self.run_fnox("get", "TEST_SECRET", env=wrong)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("different context", " ".join(result.stderr.replace("│", " ").split()))

    def test_unsupported_payload_version_fails_closed(self):
        MockState.mode = "wrong-version"
        self.set_secret()
        MockState.mode = "normal"
        result = self.run_fnox("get", "TEST_SECRET")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unsupported version", " ".join(result.stderr.replace("│", " ").split()))

    def test_malformed_reference_fails_closed(self):
        self.set_secret()
        content = self.config.read_text(encoding="utf-8")
        self.config.write_text(content.replace("nexusq-fnox:v1:", "unknown-format:", 1), encoding="utf-8")
        result = self.run_fnox("get", "TEST_SECRET")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("unsupported or malformed", " ".join(result.stderr.replace("│", " ").split()))

    def test_authentication_failure_is_reported(self):
        bad = self.env.copy()
        bad["NEXUSQ_TOKEN"] = "X" * 40
        result = self.run_fnox("provider", "test", "nexusq", env=bad)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("authentication failed", " ".join(result.stderr.replace("│", " ").split()))
        self.assertNotIn(bad["NEXUSQ_TOKEN"], result.stderr)

    def test_locked_vault_fails_closed(self):
        self.set_secret()
        MockState.mode = "locked"
        result = self.run_fnox("get", "TEST_SECRET")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("vault is locked", " ".join(result.stderr.replace("│", " ").split()))

    def test_post_redirect_is_not_followed_even_with_hostile_curlrc(self):
        MockState.mode = "redirect"
        (self.project / ".curlrc").write_text("location\ninsecure\n", encoding="utf-8")
        hostile_home = self.env.copy()
        hostile_home["HOME"] = str(self.project)
        result = self.run_fnox("set", "REDIRECT_SECRET", "--provider", "nexusq", input_text="do-not-forward", env=hostile_home)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(MockState.redirect_captures, 0)

    def test_chunked_response_is_bounded(self):
        self.set_secret()
        MockState.mode = "oversized"
        result = self.run_fnox("get", "TEST_SECRET")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("curl transport failed", " ".join(result.stderr.replace("│", " ").split()))

    def test_url_with_path_is_rejected(self):
        invalid = self.env.copy()
        invalid["NEXUSQ_URL"] = self.url + "/api"
        result = self.run_fnox("provider", "test", "nexusq", env=invalid)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("origin URL", " ".join(result.stderr.replace("│", " ").split()))

    def test_remote_plain_http_is_rejected(self):
        remote = self.env.copy()
        remote["NEXUSQ_URL"] = "http://example.com"
        result = self.run_fnox("provider", "test", "nexusq", env=remote)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("must use HTTPS", " ".join(result.stderr.replace("│", " ").split()))


if __name__ == "__main__":
    unittest.main(verbosity=2)
