FROM maven:3.10-eclipse-temurin-21-alpine@sha256:db0ebf0cb6f5f9e777250b0a7ee5359167f023d9cd1ed9508266c926234c32a8 as build

# Set working directory
WORKDIR /usr/src/myapp

# Resolve all the dependencies and cache them to save a LOT of time
COPY pom.xml .
RUN mvn dependency:resolve dependency:resolve-plugins

# Build the application, usually only this part gets rebuilt locally, use offline mode and skip tests
COPY src ./src
RUN mvn clean package -DskipTests

# The final image should have minimal layers
FROM eclipse-temurin:27-jre-alpine@sha256:3689ae1ed2051837cfd9ad0977446a60ca799d95cbb8f8e64a42e72ecaad6b35
RUN apk add --no-cache curl
COPY --from=build /usr/src/myapp/target/kafka-avro-0.0.1-SNAPSHOT.jar app.jar
ENTRYPOINT ["java", "-jar", "app.jar"]