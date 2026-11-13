FROM maven:3.10-eclipse-temurin-21-alpine@sha256:b82c5423496a4ac8df790adb5a5673c296692fe8c868c2a5ad94a11f2ad83cdd as build

# Set working directory
WORKDIR /usr/src/myapp

# Resolve all the dependencies and cache them to save a LOT of time
COPY pom.xml .
RUN mvn dependency:resolve dependency:resolve-plugins

# Build the application, usually only this part gets rebuilt locally, use offline mode and skip tests
COPY src ./src
RUN mvn clean package -DskipTests

# The final image should have minimal layers
FROM eclipse-temurin:27-jre-alpine@sha256:a0f3b523fd89e242ce302715ce367f68e9073c59274e65746d0791c750f38de3
RUN apk add --no-cache curl
COPY --from=build /usr/src/myapp/target/kafka-avro-0.0.1-SNAPSHOT.jar app.jar
ENTRYPOINT ["java", "-jar", "app.jar"]