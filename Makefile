# ZephyrCache Deploy Makefile
#
# Usage:
#   make up                  # gossip mode, 3 nodes (default)
#   make up DISCOVERY=etcd   # etcd mode, 3 nodes
#   make up NODES=5          # gossip mode, 5 nodes
#   make down                # tear down
#   make logs                # tail all logs
#   make status              # show running containers
#   make format              # format all code

DISCOVERY ?= gossip
NODES     ?= 3
PROJECT   := zephyr
COMPOSE   := docker compose -p $(PROJECT) -f deploy/docker-compose.yml -f deploy/docker-compose.$(DISCOVERY).yml
ALL := docker compose -p $(PROJECT) -f deploy/docker-compose.yml -f deploy/docker-compose.etcd.yml -f deploy/docker-compose.gossip.yml

.PHONY: up down restart build logs status clean ps

## seed: start seed node
seed: build
	$(COMPOSE) up -d seed

## up: scale peers
up: build
	$(COMPOSE) up -d --scale node=$(NODES)

## down: stop and remove containers
down:
	$(ALL) down

## restart: full restart
restart: down up

## build: rebuild the node image
build:
	$(COMPOSE) build

## logs: tail logs from all containers
logs:
	$(COMPOSE) logs -f

## logs-node: tail logs from nodes only
logs-node:
	$(COMPOSE) logs -f node

## status: show running containers and health
status:
	$(COMPOSE) ps

## clean: tear down and remove volumes
clean:
	$(ALL) down -v --remove-orphans

test:
	go test ./... -v

bench:
	go test -bench=BenchmarkPutGet -benchtime=5s -run=^$ -v ./cmd/bench -- -req=50000

format:
	go fmt ./...
	gofmt -s -w .

## help: show available targets
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
