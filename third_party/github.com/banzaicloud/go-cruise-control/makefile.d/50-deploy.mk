##@ Development environment

DOCKER_COMPOSE_PROJECT_NAME = go-cruise-control
DEPLOY_DIR ?= $(PROJECT_DIR)/deploy
COMPOSE_PROFILES ?=
DOCKER_COMPOSE_TIMEOUT ?= 60

export COMPOSE_PROFILES

.PHONY: start
start: ## Spin up local development environment
	$(info *** Spinning up local development environment...)
	@docker compose \
		--project-name "$(DOCKER_COMPOSE_PROJECT_NAME)" \
		--project-directory "$(DEPLOY_DIR)" \
		up \
		--detach \
		--remove-orphans \
		-t $(DOCKER_COMPOSE_TIMEOUT) \
		--wait

.PHONY: stop
stop: ## Stop local development environment
	$(info *** Stopping local development environment...)
	@docker compose \
		--project-name "$(DOCKER_COMPOSE_PROJECT_NAME)" \
		--project-directory "$(DEPLOY_DIR)" \
		down \
		--remove-orphans \
		--volumes \
		-t $(DOCKER_COMPOSE_TIMEOUT)
