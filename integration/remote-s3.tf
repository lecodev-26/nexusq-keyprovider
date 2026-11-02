terraform {
  backend "s3" {
    bucket = "nexusq-e2e-state"
    key    = "state/terraform.tfstate"
    region = "us-east-1"

    endpoints = {
      s3 = "http://127.0.0.1:9090"
    }

    skip_credentials_validation = true
    skip_region_validation      = true
    skip_metadata_api_check     = true
    skip_requesting_account_id  = true
    skip_s3_checksum            = true
    use_path_style              = true
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

resource "terraform_data" "s3_demo" {
  input = "nexusq-s3-plaintext-marker"
}
