# Example: hand off a Terraform-generated database password to a Northflank
# secret group so services in the project can consume it as an env var.

terraform {
  required_providers {
    northflank = {
      source  = "vestmark-infra/northflank"
      version = "~> 0.1"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }
}

provider "northflank" {
  # api_token resolved from NORTHFLANK_API_TOKEN
}

# Generate a random database password with Terraform.
resource "random_password" "db_password" {
  length           = 32
  special          = true
  override_special = "!#$%^&*()-_=+[]{}<>:?"
}

# Hand it off to Northflank as a secret group.
resource "northflank_secret_group" "app_secrets" {
  project_id  = "my-project"
  name        = "App Secrets"
  secret_type = "environment"
  priority    = 10

  variables = {
    DB_PASSWORD = random_password.db_password.result
    DB_HOST     = "db.internal"
    DB_PORT     = "5432"
  }

  files = {
    "/etc/app/config.json" = {
      data     = base64encode("{\"env\": \"production\"}")
      encoding = "utf-8"
    }
  }

  docker_secret_mounts = {
    "tls-cert" = {
      data     = base64encode("--- certificate content ---")
      encoding = "utf-8"
    }
  }
}

# Outputs for cross-module reference.
output "secret_group_id" {
  value       = northflank_secret_group.app_secrets.id
  description = "ID of the Northflank secret group."
}
