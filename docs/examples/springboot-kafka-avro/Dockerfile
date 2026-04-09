FROM maven:3.9-eclipse-temurin-21-alpine@sha256:eb9561832b985fe1c341af2c147412810c3cad0ddb250e6de31e0f1300433bb1 as build

# Set working directory
WORKDIR /usr/src/myapp

# Resolve all the dependencies and cache them to save a LOT of time
COPY pom.xml .
RUN mvn dependency:resolve dependency:resolve-plugins

# Build the application, usually only this part gets rebuilt locally, use offline mode and skip tests
COPY src ./src
RUN mvn clean package -DskipTests

# The final image should have minimal layers
FROM eclipse-temurin:26-jre-alpine@sha256:4b869106a967d8bd651c9fba1b2030ea50684ec72739ddaf8a5b728c7dfa7926
RUN apk add --no-cache curl
COPY --from=build /usr/src/myapp/target/kafka-avro-0.0.1-SNAPSHOT.jar app.jar
ENTRYPOINT ["java", "-jar", "app.jar"]