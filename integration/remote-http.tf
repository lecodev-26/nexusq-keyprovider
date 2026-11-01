terraform {
  backend "http" {
    address        = "http://127.0.0.1:18444/state"
    lock_address   = "http://127.0.0.1:18444/lock"
    unlock_address = "http://127.0.0.1:18444/lock"
    lock_method    = "LOCK"
    unlock_method  = "UNLOCK"
    retry_wait_min = 1
  }

  encryption {
    key_provider "external" "nexusq" {
      command = ["/absolute/path/to/nexusq-keyprovider"]
    }

    method "aes_gcm" "nexusq" {
      keys = key_provider.external.nexusq
    }

    state {
      method   = method.aes_gcm.nexusq
      enforced = true
    }

    plan {
      method   = method.aes_gcm.nexusq
      enforced = true
    }
  }
}

resource "terraform_data" "remote_demo" {
  input = "replace-me"
}
