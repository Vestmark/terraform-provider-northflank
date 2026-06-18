data "northflank_team" "main" {
  name = "Acme Engineering"
}

data "northflank_project" "api" {
  name    = "api-service"
  team_id = data.northflank_team.main.id
}

output "project_id" {
  value = data.northflank_project.api.id
}
