terraform {
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

resource "terraform_data" "example" {
  input = "replace-me"
}

output "example_value" {
  value = terraform_data.example.output
}
