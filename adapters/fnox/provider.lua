-- NEXUS-Q provider for fnox (Lua 5.4).
-- Secrets are sent only to the configured NEXUS-Q endpoint over curl.
-- Do not replace this transport with fnox.http: that host follows redirects.

local json = require("fnox.json")
local base64 = require("fnox.base64")
local cmd = require("fnox.cmd")
local fnox = require("fnox")

local PREFIX = "nexusq-fnox:v1:"
local METADATA = "nexusq-keyprovider-fnox-v1"
local MAX_REQUEST_BYTES = 1024 * 1024
local MAX_RESPONSE_BYTES = 2 * 1024 * 1024
local CONNECT_TIMEOUT_SECONDS = 5
local TOTAL_TIMEOUT_SECONDS = 30

local curl_version_checked = false

local function fail(message)
  error("nexusq-fnox: " .. message, 0)
end

local function required_env(name, maximum)
  local value = os.getenv(name)
  if type(value) ~= "string" or value == "" then
    fail("required environment variable " .. name .. " is not set")
  end
  if #value > maximum or value:find("[%c]") then
    fail("environment variable " .. name .. " has an invalid value")
  end
  return value
end

local function validate_host(host)
  if host:sub(1, 1) == "[" then
    local inner = host:sub(2, -2)
    if not inner:find(":", 1, true) or inner:find("[^%x:%.]") then
      fail("NEXUSQ_URL contains an invalid IPv6 host")
    end
    return inner:lower() == "::1"
  end

  if host:find("[^%w%.%-]") or host:find("%.%.") or #host > 253 or host:sub(1, 1) == "." or host:sub(-1) == "." then
    fail("NEXUSQ_URL contains an invalid host")
  end

  for label in host:gmatch("[^%.]+") do
    if #label > 63 or label:sub(1, 1) == "-" or label:sub(-1) == "-" then
      fail("NEXUSQ_URL contains an invalid host")
    end
  end

  local lower = host:lower()
  if lower == "localhost" then return true end
  local a, b, c, d = lower:match("^(%d+)%.(%d+)%.(%d+)%.(%d+)$")
  if a then
    a, b, c, d = tonumber(a), tonumber(b), tonumber(c), tonumber(d)
    if a > 255 or b > 255 or c > 255 or d > 255 then
      fail("NEXUSQ_URL contains an invalid IPv4 address")
    end
    return a == 127
  end
  return false
end

local function base_url()
  local configured = required_env("NEXUSQ_URL", 2048)
  -- Keep the accepted grammar intentionally narrow: no paths, query, fragment,
  -- userinfo, percent-encoding, whitespace or backslashes.
  local scheme, authority = configured:match("^(https?)://([^/]+)/?$")
  if not scheme or authority:sub(-1) == ":" or authority:find("[@?#\\%s]") then
    fail("NEXUSQ_URL must be an origin URL without credentials, path, query or fragment")
  end

  local host, port
  if authority:sub(1, 1) == "[" then
    host, port = authority:match("^(%[[%x:%.]+%]):?(%d*)$")
  else
    host, port = authority:match("^([%w%.%-]+):?(%d*)$")
  end
  if not host then fail("NEXUSQ_URL has an invalid authority") end
  if port ~= "" then
    local number = tonumber(port)
    if not number or number < 1 or number > 65535 then
      fail("NEXUSQ_URL has an invalid port")
    end
  end

  local loopback = validate_host(host)
  if scheme == "http" and not loopback then
    fail("NEXUSQ_URL must use HTTPS unless it targets a loopback address")
  end
  return scheme .. "://" .. authority, scheme
end

local function configuration()
  local url, scheme = base_url()
  local token = required_env("NEXUSQ_TOKEN", 8192)
  if #token < 32 or token:find("%s") then
    fail("NEXUSQ_TOKEN must contain at least 32 non-whitespace bytes")
  end
  local key_id = required_env("NEXUSQ_KEY_ID", 256)
  local context_id = required_env("NEXUSQ_CONTEXT_ID", 256)
  return {
    url = url,
    scheme = scheme,
    token = token,
    key_id = key_id,
    context_id = context_id,
  }
end

local function curl_quote(value)
  value = value:gsub("\\", "\\\\")
    :gsub('"', '\\"')
    :gsub("\r", "\\r")
    :gsub("\n", "\\n")
    :gsub("\t", "\\t")
  return '"' .. value .. '"'
end

local function ensure_curl()
  if curl_version_checked then return end
  local result = cmd.run({ "curl", "--disable", "--version" })
  if not result or not result.ok then
    fail("curl is required but could not be executed")
  end
  if not result.stdout:match("^curl%s+%d+%.%d+%.%d+") then
    fail("could not determine the installed curl version")
  end
  local major, minor = result.stdout:match("^curl%s+(%d+)%.(%d+)")
  major, minor = tonumber(major), tonumber(minor)
  if major < 8 or (major == 8 and minor < 4) then
    fail("curl 8.4.0 or newer is required for bounded chunked responses")
  end
  curl_version_checked = true
end

local function http_request(config, method, path, request_body)
  ensure_curl()
  if request_body and #request_body > MAX_REQUEST_BYTES then
    fail("NEXUS-Q request exceeds the 1 MiB request-body limit")
  end

  local lines = {
    "silent",
    "url = " .. curl_quote(config.url .. path),
    "request = " .. curl_quote(method),
    "proto = " .. curl_quote(config.scheme == "https" and "=https" or "=http"),
    "connect-timeout = " .. tostring(CONNECT_TIMEOUT_SECONDS),
    "max-time = " .. tostring(TOTAL_TIMEOUT_SECONDS),
    "max-filesize = " .. tostring(MAX_RESPONSE_BYTES),
    "header = " .. curl_quote("Authorization: Bearer " .. config.token),
    "header = " .. curl_quote("Accept: application/json"),
    "write-out = " .. curl_quote("%{http_code}"),
  }
  if config.scheme == "http" then
    lines[#lines + 1] = "noproxy = \"*\""
  end
  if request_body then
    lines[#lines + 1] = "header = " .. curl_quote("Content-Type: application/json")
    lines[#lines + 1] = "data-binary = " .. curl_quote(request_body)
  end

  local result = cmd.run({ "curl", "--disable", "--config", "-" }, {
    stdin = table.concat(lines, "\n") .. "\n",
  })
  if not result or not result.ok then
    fail("curl transport failed or the response exceeded the configured limit")
  end

  local body, status_text = result.stdout:match("^(.*)(%d%d%d)$")
  if not status_text then
    fail("NEXUS-Q returned an incomplete HTTP response")
  end
  if #body > MAX_RESPONSE_BYTES then
    fail("NEXUS-Q response exceeds the 2 MiB response limit")
  end

  local status = tonumber(status_text)
  if status == 401 or status == 403 then
    fnox.auth_error("NEXUS-Q authentication failed; check NEXUSQ_TOKEN")
  elseif status == 423 then
    fail("NEXUS-Q vault is locked")
  elseif status < 200 or status >= 300 then
    fail("NEXUS-Q request failed with HTTP status " .. status_text)
  end
  return body
end

local function decode_response(body)
  local ok, decoded = pcall(json.decode, body)
  if not ok or type(decoded) ~= "table" or type(decoded.data) ~= "string" then
    fail("NEXUS-Q returned an invalid JSON response")
  end
  if decoded.data == "" or decoded.data:find("[^A-Za-z0-9+/=]") then
    fail("NEXUS-Q returned invalid Base64 data")
  end
  local decoded_ok, bytes = pcall(base64.decode, decoded.data)
  if not decoded_ok or type(bytes) ~= "string" or base64.encode(bytes) ~= decoded.data then
    fail("NEXUS-Q returned invalid Base64 data")
  end
  return decoded.data, bytes
end

local function encrypt(config, value)
  if type(value) ~= "string" then
    fail("fnox encrypt hook requires a string value")
  end
  if #value > MAX_REQUEST_BYTES then
    fail("secret value exceeds the 1 MiB NEXUS-Q request-body limit")
  end
  local payload = json.encode({
    adapter = "nexusq-fnox",
    version = "1",
    context_id = config.context_id,
    secret = value,
  })
  local request_body = json.encode({
    key_id = config.key_id,
    plaintext = payload,
    metadata = METADATA,
  })
  local response = http_request(config, "POST", "/v1/encrypt", request_body)
  local encoded_envelope, raw_envelope = decode_response(response)
  if #raw_envelope == 0 then fail("NEXUS-Q returned an empty envelope") end

  local reference = PREFIX .. encoded_envelope
  local decrypt_request = json.encode({ envelope = encoded_envelope })
  if #decrypt_request > MAX_REQUEST_BYTES then
    fail("NEXUS-Q envelope is too large to be recovered under the 1 MiB request limit")
  end
  return reference
end

local function get(config, reference)
  if type(reference) ~= "string" or reference:sub(1, #PREFIX) ~= PREFIX then
    fail("secret reference has an unsupported or malformed NEXUS-Q fnox format")
  end
  local encoded_envelope = reference:sub(#PREFIX + 1)
  if #reference > MAX_REQUEST_BYTES or encoded_envelope == "" or encoded_envelope:find("[^A-Za-z0-9+/=]") then
    fail("secret reference contains invalid Base64 envelope data")
  end
  local decoded_ok, raw_envelope = pcall(base64.decode, encoded_envelope)
  if not decoded_ok or type(raw_envelope) ~= "string" or #raw_envelope == 0 or base64.encode(raw_envelope) ~= encoded_envelope then
    fail("secret reference contains invalid Base64 envelope data")
  end

  local request_body = json.encode({ envelope = encoded_envelope })
  if #request_body > MAX_REQUEST_BYTES then
    fail("secret reference exceeds the NEXUS-Q request-body limit")
  end
  local response = http_request(config, "POST", "/v1/decrypt", request_body)
  local _, plaintext = decode_response(response)
  local payload_ok, payload = pcall(json.decode, plaintext)
  if not payload_ok or type(payload) ~= "table" then
    fail("decrypted NEXUS-Q payload is not valid JSON")
  end

  local allowed = { adapter = true, version = true, context_id = true, secret = true }
  local count = 0
  for key in pairs(payload) do
    if not allowed[key] then fail("decrypted NEXUS-Q payload contains an unknown field") end
    count = count + 1
  end
  if count ~= 4 then
    fail("decrypted NEXUS-Q payload has an unexpected field count")
  end
  if payload.adapter ~= "nexusq-fnox" then
    fail("decrypted NEXUS-Q payload has an unsupported adapter identifier")
  end
  if payload.version ~= "1" then
    fail("decrypted NEXUS-Q payload has an unsupported version")
  end
  if type(payload.context_id) ~= "string" or payload.context_id ~= config.context_id then
    fail("decrypted NEXUS-Q payload belongs to a different context")
  end
  if type(payload.secret) ~= "string" then
    fail("decrypted NEXUS-Q payload does not contain a string secret")
  end
  return payload.secret
end

local function test_provider(config)
  local body = http_request(config, "GET", "/v1/keys")
  local ok, keys = pcall(json.decode, body)
  if not ok or type(keys) ~= "table" then
    fail("NEXUS-Q returned an invalid key-list response")
  end
  for _, key in ipairs(keys) do
    if type(key) == "table" and key.key_id == config.key_id then
      return true
    end
  end
  fail("configured NEXUSQ_KEY_ID was not found in NEXUS-Q")
end

return {
  encrypt = function(ctx, value)
    return encrypt(configuration(), value)
  end,
  get = function(ctx, reference)
    return get(configuration(), reference)
  end,
  test = function(ctx)
    return test_provider(configuration())
  end,
}
