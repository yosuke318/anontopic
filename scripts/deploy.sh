#!/usr/bin/env bash
# ECS で動く API を新しいイメージに入れ替える。GitHub Actions のデプロイが呼ぶほか、
# 手元から AWS の認証情報を付けて呼んでもよい。
#
#   scripts/deploy.sh present   <env>          環境が作られていれば 0、無ければ 1 で終わる
#   scripts/deploy.sh has-image <env> <tag>    ECR にそのタグのイメージがあれば 0、無ければ 1 で終わる
#   scripts/deploy.sh register  <env> <tag>    最新のタスク定義のイメージを差し替えて登録し、ARN を出す
#   scripts/deploy.sh migrate   <env> <arn>    そのタスク定義でマイグレーションを流し、終わるまで待つ
#   scripts/deploy.sh update    <env> <arn>    サービスをそのタスク定義に切り替え、終わるまで待つ
#
# 公開リポジトリの Actions のログは誰でも読める。アプリのログや接続先は出さず、
# CloudWatch Logs のどこを見ればよいかだけを出す。
set -euo pipefail

project=anontopic
container=api

# サービスの切り替えを待つ上限。登録解除の遅延（300 秒）と新しいタスクの起動を含めて収まる長さにする。
update_timeout_seconds=1800
poll_interval_seconds=15

die() {
	echo "$*" >&2
	exit 1
}

usage() {
	sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//' >&2
	exit 2
}

names() {
	env=$1
	case $env in
	dev | prod) ;;
	*) die "環境は dev か prod を指定する: $env" ;;
	esac
	cluster="$project-$env"
	service="$project-$env-api"
	family="$project-$env-api"
	repository="$project-$env-api"
	log_group="/ecs/$project-$env-api"
}

present() {
	names "$1"
	local clusters services
	clusters=$(aws ecs describe-clusters --clusters "$cluster" \
		--query 'length(clusters[?status==`ACTIVE`])' --output text)
	[ "$clusters" = 1 ] || return 1
	services=$(aws ecs describe-services --cluster "$cluster" --services "$service" \
		--query 'length(services[?status==`ACTIVE`])' --output text)
	[ "$services" = 1 ]
}

has_image() {
	names "$1"
	aws ecr describe-images --repository-name "$repository" --image-ids "imageTag=$2" >/dev/null 2>&1
}

# Terraform が変えたタスク定義（環境変数や秘密の追加）も取り込めるよう、ファミリーの最新の
# リビジョンを元にする。
register() {
	names "$1"
	local tag=$2 uri current input
	has_image "$env" "$tag" || die "ECR の $repository にタグ $tag のイメージが無い"

	uri=$(aws ecr describe-repositories --repository-names "$repository" \
		--query 'repositories[0].repositoryUri' --output text)
	current=$(aws ecs describe-task-definition --task-definition "$family" --include TAGS --output json)

	input=$(jq --arg container "$container" --arg image "$uri:$tag" '
		.tags as $tags
		| .taskDefinition
		| .containerDefinitions |= map(if .name == $container then .image = $image else . end)
		| del(.taskDefinitionArn, .revision, .status, .requiresAttributes, .compatibilities,
			.registeredAt, .registeredBy, .deregisteredAt)
		| if ($tags | length) > 0 then .tags = $tags else . end
	' <<<"$current")

	aws ecs register-task-definition --cli-input-json "$input" \
		--query 'taskDefinition.taskDefinitionArn' --output text
}

# サービスと同じサブネットとセキュリティグループで、コマンドだけを /migrate up に替えて 1 回動かす。
migrate() {
	names "$1"
	local arn=$2 network overrides started task task_id result exit_code reason
	network=$(aws ecs describe-services --cluster "$cluster" --services "$service" \
		--query 'services[0].networkConfiguration' --output json)
	overrides=$(jq -n --arg container "$container" \
		'{containerOverrides: [{name: $container, command: ["/migrate", "up"]}]}')

	started=$(aws ecs run-task --cluster "$cluster" --task-definition "$arn" \
		--launch-type FARGATE --network-configuration "$network" --overrides "$overrides" \
		--started-by deploy-migrate --output json)
	task=$(jq -r '.tasks[0].taskArn // empty' <<<"$started")
	[ -n "$task" ] || die "マイグレーションのタスクを起動できなかった: $(jq -c '.failures' <<<"$started")"
	task_id=${task##*/}
	echo "マイグレーションのタスク $task_id を起動した。終わるまで待つ。"

	aws ecs wait tasks-stopped --cluster "$cluster" --tasks "$task"

	result=$(aws ecs describe-tasks --cluster "$cluster" --tasks "$task" --output json)
	exit_code=$(jq -r --arg container "$container" \
		'.tasks[0].containers[] | select(.name == $container) | .exitCode // "none"' <<<"$result")
	reason=$(jq -r '.tasks[0].stoppedReason // ""' <<<"$result")

	if [ "$exit_code" != 0 ]; then
		echo "マイグレーションが失敗した（終了コード: $exit_code、理由: $reason）。" >&2
		die "ログは CloudWatch Logs の $log_group にあるストリーム $container/$container/$task_id を見る。"
	fi
	echo "マイグレーションが終わった。"
}

# ECS のサーキットブレーカーは、新しいタスクが起動しないと元のタスク定義に戻す。戻った後も
# サービスは安定状態になるため、待つのはサービスではなく今回のデプロイの結果にする。
update() {
	names "$1"
	local arn=$2 deployment state waited=0
	deployment=$(aws ecs update-service --cluster "$cluster" --service "$service" \
		--task-definition "$arn" \
		--query 'service.deployments[?status==`PRIMARY`] | [0].id' --output text)
	echo "サービスを ${arn##*/} に切り替える。"

	while :; do
		state=$(aws ecs describe-services --cluster "$cluster" --services "$service" \
			--query "services[0].deployments[?id=='$deployment'] | [0].rolloutState" --output text)
		case $state in
		COMPLETED)
			echo "切り替えが終わった。"
			return 0
			;;
		FAILED | None)
			die "新しいタスクが正常にならず、ECS が元のタスク定義に戻した。"
			;;
		esac

		[ "$waited" -lt "$update_timeout_seconds" ] ||
			die "切り替えが ${update_timeout_seconds} 秒で終わらなかった（状態: $state）。"
		sleep "$poll_interval_seconds"
		waited=$((waited + poll_interval_seconds))
	done
}

[ $# -ge 2 ] || usage
command=$1
shift

case $command in
present) [ $# -eq 1 ] || usage; present "$@" ;;
has-image) [ $# -eq 2 ] || usage; has_image "$@" ;;
register) [ $# -eq 2 ] || usage; register "$@" ;;
migrate) [ $# -eq 2 ] || usage; migrate "$@" ;;
update) [ $# -eq 2 ] || usage; update "$@" ;;
*) usage ;;
esac
