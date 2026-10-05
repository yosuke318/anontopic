resource "aws_ecr_repository" "web" {
  name = "${var.name_prefix}-web"

  # 同じタグで別のイメージを上書きできないようにし、タグからイメージを一意に引けるようにする。
  image_tag_mutability = "IMMUTABLE"

  force_delete = var.ecr_force_delete

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_lifecycle_policy" "web" {
  repository = aws_ecr_repository.web.name

  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep the 30 most recent images"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 30
      }
      action = {
        type = "expire"
      }
    }]
  })
}
