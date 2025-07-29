# create a makefile that build the kafka image from the Dockerfile multiarch for amd64 and arm64
build:
	- docker buildx create --use --name kafka_builder
	docker buildx use kafka_builder
	docker buildx build --push --platform linux/amd64,linux/arm64 -t docker-pipeline-upstream-mirror.dr-uw2.adobeitc.com/adobe/kafka:3.9.1 .
	- docker buildx rm kafka_builder
