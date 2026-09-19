FROM maven:3.9-eclipse-temurin-21-alpine@sha256:d66dd3fe25f08b228cf13ce4a5b0aebceb1f609e898a7fa0a7c4997ff82f283c as build

# Set working directory
WORKDIR /usr/src/myapp

# Resolve all the dependencies and cache them to save a LOT of time
COPY pom.xml .
RUN mvn dependency:resolve dependency:resolve-plugins

# Build the application, usually only this part gets rebuilt locally, use offline mode and skip tests
COPY src ./src
RUN mvn clean package -DskipTests

# The final image should have minimal layers
FROM eclipse-temurin:26-jre-alpine@sha256:2db9a5fb7c52fb44f9ffcf2d5caa16a39d4e8eb2977045ef3bb6589ae1c63ef5
RUN apk add --no-cache curl
COPY --from=build /usr/src/myapp/target/kafka-avro-0.0.1-SNAPSHOT.jar app.jar
ENTRYPOINT ["java", "-jar", "app.jar"]