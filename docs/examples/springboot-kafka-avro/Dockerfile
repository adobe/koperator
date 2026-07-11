FROM maven:3.9-eclipse-temurin-21-alpine@sha256:d88e5b38297858f65f97bc7e7964c760ab988fd18ace41589176f1468c49a489 as build

# Set working directory
WORKDIR /usr/src/myapp

# Resolve all the dependencies and cache them to save a LOT of time
COPY pom.xml .
RUN mvn dependency:resolve dependency:resolve-plugins

# Build the application, usually only this part gets rebuilt locally, use offline mode and skip tests
COPY src ./src
RUN mvn clean package -DskipTests

# The final image should have minimal layers
FROM eclipse-temurin:26-jre-alpine@sha256:c4a22bec4f4368636abb9b6fe2b2350fd7fae1ec0d3bf43fcaae1be720c3bbd1
RUN apk add --no-cache curl
COPY --from=build /usr/src/myapp/target/kafka-avro-0.0.1-SNAPSHOT.jar app.jar
ENTRYPOINT ["java", "-jar", "app.jar"]