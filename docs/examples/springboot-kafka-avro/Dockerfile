FROM maven:3.10-eclipse-temurin-21-alpine@sha256:6e5df865a159b4e738ea7c330c069f872eecfd4e978963720f8b0264338c300d as build

# Set working directory
WORKDIR /usr/src/myapp

# Resolve all the dependencies and cache them to save a LOT of time
COPY pom.xml .
RUN mvn dependency:resolve dependency:resolve-plugins

# Build the application, usually only this part gets rebuilt locally, use offline mode and skip tests
COPY src ./src
RUN mvn clean package -DskipTests

# The final image should have minimal layers
FROM eclipse-temurin:27-jre-alpine@sha256:51f686a5322c0b762dff27e8e94b4b6582d7e71321d3a23b1f3c53b4078aabc5
RUN apk add --no-cache curl
COPY --from=build /usr/src/myapp/target/kafka-avro-0.0.1-SNAPSHOT.jar app.jar
ENTRYPOINT ["java", "-jar", "app.jar"]