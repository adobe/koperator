FROM maven:3.9-eclipse-temurin-21-alpine@sha256:adae65922fda0826185289ae9829d961662de5390c0190acd57b188243e3ce23 as build

# Set working directory
WORKDIR /usr/src/myapp

# Resolve all the dependencies and cache them to save a LOT of time
COPY pom.xml .
RUN mvn dependency:resolve dependency:resolve-plugins

# Build the application, usually only this part gets rebuilt locally, use offline mode and skip tests
COPY src ./src
RUN mvn clean package -DskipTests

# The final image should have minimal layers
FROM eclipse-temurin:26-jre-alpine@sha256:bae1c79cfc2787461c7d1ead6f41b67e3d75cc4b372dfe45b57432f9460d8ff3
RUN apk add --no-cache curl
COPY --from=build /usr/src/myapp/target/kafka-avro-0.0.1-SNAPSHOT.jar app.jar
ENTRYPOINT ["java", "-jar", "app.jar"]