FROM maven:3.9-eclipse-temurin-21-alpine@sha256:98c64ec556736acfff08a3b49c9ccfb3b2c3704c9deeee81ec2a18ba6fd059f2 as build

# Set working directory
WORKDIR /usr/src/myapp

# Resolve all the dependencies and cache them to save a LOT of time
COPY pom.xml .
RUN mvn dependency:resolve dependency:resolve-plugins

# Build the application, usually only this part gets rebuilt locally, use offline mode and skip tests
COPY src ./src
RUN mvn clean package -DskipTests

# The final image should have minimal layers
FROM eclipse-temurin:25-jre-alpine@sha256:b51543f89580c1ba70e441cfbc0cfc1635c3c16d2e2d77fec9d890342a3a8687
RUN apk add --no-cache curl
COPY --from=build /usr/src/myapp/target/kafka-avro-0.0.1-SNAPSHOT.jar app.jar
ENTRYPOINT ["java", "-jar", "app.jar"]