DOCKER_IMAGE ?= envoyproxy/envoy:v1.27.0-openshift-v2 

docker-buildx:
	@echo "Building Docker image for ${DOCKER_IMAGE}"
	- docker buildx create --name envoy-project-builder
	docker buildx use envoy-project-builder
	docker buildx build --platform linux/amd64,linux/arm64 --tag ${DOCKER_IMAGE} --push .
	- docker buildx rm envoy-project-builder
