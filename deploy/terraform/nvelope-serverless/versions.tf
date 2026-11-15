terraform {
  required_version = ">= 1.7"
  required_providers {
    yandex = {
      source  = "yandex-cloud/yandex"
      version = "~> 0.248"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.9"
    }
  }
}

# Credentials come from the environment (YC_TOKEN or
# YC_SERVICE_ACCOUNT_KEY_FILE), as for any yandex provider run.
provider "yandex" {
  folder_id = var.folder_id
}
