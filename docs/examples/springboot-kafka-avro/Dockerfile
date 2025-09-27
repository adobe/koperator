FROM maven:3.8-jdk-11-slim@sha256:2cb7c73ba2fd0f7ae64cfabd99180030ec85841a1197b4ae821d21836cb0aa3b as build

# Resolve all the dependencies and cache them to save a LOT of time
COPY pom.xml /usr/src/myapp/
RUN mvn -f /usr/src/myapp/pom.xml dependency:resolve dependency:resolve-plugins

# Build the application, usually only this part gets rebuilt locally, use offline mode and skip tests
COPY src /usr/src/myapp/src
RUN mvn -f /usr/src/myapp/pom.xml clean package -DskipTests

# The final image should have minimal layers
FROM openjdk:11-jre-slim@sha256:93af7df2308c5141a751c4830e6b6c5717db102b3b31f012ea29d842dc4f2b02
RUN apt-get update && apt-get install curl -y
COPY --from=build /usr/src/myapp/target/kafka-avro-0.0.1-SNAPSHOT.jar app.jar
ENTRYPOINT java -jar app.jar