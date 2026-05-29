.PHONY: build build-all proto test test-race test-cardutil \
        docker-up docker-down run run-user run-game \
        clean fmt lint deps coverage tidy \
        dev-up dev-down

# ═══════════════════════════════════════════════
# 构建
# ═══════════════════════════════════════════════

# 构建 game-service
build:
	go build -ldflags="-s -w" -o bin/game-service ./app/game/cmd/server

# 构建 user-api
build-user:
	go build -ldflags="-s -w" -o bin/user-api ./app/user

# 全量构建
build-all: build build-user

# ═══════════════════════════════════════════════
# 代码生成
# ═══════════════════════════════════════════════

# 生成 Protobuf 代码
proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/common.proto proto/messages.proto

# 生成 user-api 代码
gen-user-api:
	cd app/user && goctl api go -api user.api -dir . -style gozero

# ═══════════════════════════════════════════════
# 测试
# ═══════════════════════════════════════════════

# 全量测试（含竞态检测）
test:
	go test -v -race ./pkg/... ./app/...

# 牌型库专项测试
test-cardutil:
	go test -v -race -cover ./pkg/cardutil/...

# 覆盖率
coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# ═══════════════════════════════════════════════
# 运行
# ═══════════════════════════════════════════════

# 运行 game-service
run:
	go run ./app/game/cmd/server -f app/game/etc/game-local.yaml

# 运行 user-api
run-user:
	go run ./app/user -f app/user/etc/user-api.yaml

# 运行 game-service（集群模式实例 1）
run-cluster-1:
	go run ./app/game/cmd/server -f app/game/etc/game-prod.yaml \
		--cluster.enabled=true --cluster.host=127.0.0.1 --cluster.port=8081

# 运行 game-service（集群模式实例 2）
run-cluster-2:
	go run ./app/game/cmd/server -f app/game/etc/game-prod.yaml \
		--cluster.enabled=true --cluster.host=127.0.0.1 --cluster.port=8082

# ═══════════════════════════════════════════════
# Docker
# ═══════════════════════════════════════════════

# 启动基础设施（Redis + MySQL）
docker-up:
	docker-compose up -d

# 停止
docker-down:
	docker-compose down

# 启动所有服务（含 game 实例）
dev-up:
	docker-compose up -d

# 停止并清理
dev-down:
	docker-compose down -v

# 构建 Docker 镜像
docker-build:
	docker build -t ddz-game:latest .

# ═══════════════════════════════════════════════
# 代码质量
# ═══════════════════════════════════════════════

# 格式化
fmt:
	go fmt ./...

# Lint 检查
lint:
	golangci-lint run ./...

# 依赖管理
deps:
	go mod tidy
	go mod download

# 检查 go.mod 是否 tidy
tidy:
	go mod tidy
	git diff --name-only --exit-code go.mod go.sum || \
		(echo "go.mod/go.sum is not tidy, run 'make deps'" && exit 1)

# ═══════════════════════════════════════════════
# 清理
# ═══════════════════════════════════════════════

clean:
	rm -rf bin/
	rm -f coverage.out coverage.html
	go clean -cache
