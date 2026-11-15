# The first-run setup token (POST /api/platform/signup). Letters and digits
# only, so it survives copy and paste.
resource "random_password" "setup_token" {
  length  = 40
  special = false
}

# ponytail: the TOTP key lives only here and in Terraform state. Losing the state
# replaces it on the next apply, and every enrolled TOTP secret becomes
# unreadable. Upgrade path: a lifecycle ignore_changes plus a documented rotation.
resource "random_id" "totp_key" {
  byte_length = 32
}

resource "yandex_lockbox_secret" "app" {
  name                = "nvelope"
  deletion_protection = var.deletion_protection
}

resource "yandex_lockbox_secret_version" "app" {
  secret_id = yandex_lockbox_secret.app.id
  entries {
    key        = "setup_token"
    text_value = random_password.setup_token.result
  }
  entries {
    key        = "totp_encryption_key"
    text_value = random_id.totp_key.hex
  }
  entries {
    key        = "access_key_id"
    text_value = yandex_iam_service_account_static_access_key.app.access_key
  }
  entries {
    key        = "secret_access_key"
    text_value = yandex_iam_service_account_static_access_key.app.secret_key
  }
}
