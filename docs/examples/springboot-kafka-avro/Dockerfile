FROM maven:3.10-eclipse-temurin-21-alpine@sha256:39d4a423da857d3de27e3d6d4bade31702e9fc1e3ed59cd8c96ce4ca10d3a6d6 as build

# Set working directory
WORKDIR /usr/src/myapp

# Resolve all the dependencies and cache them to save a LOT of time
COPY pom.xml .
RUN mvn dependency:resolve dependency:resolve-plugins

# Build the application, usually only this part gets rebuilt locally, use offline mode and skip tests
COPY src ./src
RUN mvn clean package -DskipTests

# The final image should have minimal layers
FROM eclipse-temurin:26-jre-alpine@sha256:9eedff2367194d11eddd6f14101b444945a708c986270cd5716b934596ba3a31
RUN apk add --no-cache curl
COPY --from=build /usr/src/myapp/target/kafka-avro-0.0.1-SNAPSHOT.jar app.jar
ENTRYPOINT ["java", "-jar", "app.jar"]