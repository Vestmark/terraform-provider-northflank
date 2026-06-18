data "northflank_secret" "shared" {
  project_id = "my-project"
  id         = "shared-secrets"
}

output "db_host" {
  value     = data.northflank_secret.shared.variables["DB_HOST"]
  sensitive = true
}
