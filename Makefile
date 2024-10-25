LDFLAGS="-w -s -X main.GitCommitId=${GIT_COMMIT_ID} -X main.BuildTime=${BUILD_TIME}"
BUILD_DIR=build
OBJ_NAME=ip2loc
APP_VERSION=v1.0

.PHONY: default
default: help

# help 提取注释作为帮助信息
help: ## Show this help.
	@fgrep -h "##" $(MAKEFILE_LIST) | fgrep -v fgrep | sed -e 's/\\$$//' | sed -e 's/##//'

## clean 删除构建目录
.PHONY: clean
clean:
	rm -rf ${BUILD_DIR}

## generate 生成源码
.PHONY: generate
generate:
	go generate ./...

--mkdir-dir:
	mkdir -p ${BUILD_DIR}

--upx:
	upx ${BUILD_DIR}/${OBJ_NAME}

## build 构建当前系统环境二进制文件
.PHONY: build
build: clean generate --mkdir-dir
	GOOS="linux" GOARCH="amd64" CGO_ENABLED=0 go build -ldflags ${LDFLAGS} -o ${BUILD_DIR}/${OBJ_NAME}

## build-upx 构建当前系统环境二进制文件 并压缩
.PHONY: build-upx
build-upx: build --upx