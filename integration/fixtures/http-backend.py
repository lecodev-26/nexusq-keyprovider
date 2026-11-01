# Development-only fixture. NOT a production HTTP state backend.
# Binds to loopback and persists one state file to disk.
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
import json

STATE = Path("/tmp/nexusq-http-backend-state.tfstate")
LOCK = Path("/tmp/nexusq-http-backend-lock.json")

class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        print(fmt % args, flush=True)

    def send(self, code, body=b"", content_type="application/json"):
        self.send_response(code)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if body:
            self.wfile.write(body)

    def do_GET(self):
        if self.path.split("?", 1)[0] != "/state":
            return self.send(404)
        if not STATE.exists():
            return self.send(404, b"state not found", "text/plain")
        self.send(200, STATE.read_bytes(), "application/octet-stream")

    def do_POST(self):
        return self._write_state()

    def do_PUT(self):
        path = self.path.split("?", 1)[0]
        if path == "/state":
            return self._write_state()
        if path == "/lock":
            body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
            if LOCK.exists():
                return self.send(423, LOCK.read_bytes())
            try:
                info = json.loads(body)
            except Exception:
                return self.send(400)
            LOCK.write_text(json.dumps(info))
            return self.send(200, json.dumps(info).encode())
        return self.send(404)

    def do_LOCK(self):
        return self.do_PUT()

    def do_DELETE(self):
        path = self.path.split("?", 1)[0]
        if path == "/state":
            if STATE.exists():
                STATE.unlink()
            return self.send(200, b"", "text/plain")
        if path == "/lock":
            body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
            try:
                info = json.loads(body) if body else {}
            except Exception:
                info = {}
            if LOCK.exists():
                current = json.loads(LOCK.read_text())
                if info.get("ID") and current.get("ID") != info.get("ID"):
                    return self.send(409, b"lock id mismatch", "text/plain")
                LOCK.unlink()
            return self.send(200, b"", "text/plain")
        return self.send(404)

    def do_UNLOCK(self):
        return self.do_DELETE()

    def _write_state(self):
        if self.path.split("?", 1)[0] != "/state":
            return self.send(404)
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        STATE.write_bytes(body)
        return self.send(200, b"", "text/plain")

HTTPServer(("127.0.0.1", 18444), Handler).serve_forever()
