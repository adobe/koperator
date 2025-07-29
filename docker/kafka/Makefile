# create a makefile that build the kafka image from the Dockerfile multiarch for amd64 and arm64
build:
	docker buildx create --use --name kafka_builder || true
	docker buildx build --platform linux/amd64,linux/arm64 -t docker-pipeline-upstream-mirror.dr-uw2.adobeitc.com/adobe/kafka:3.9.1 .
	docker buildx delete kafka_builder || true
