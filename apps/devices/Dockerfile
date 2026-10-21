FROM gradle:8.12.1-jdk21 AS build
WORKDIR /workspace
COPY docs/api/proto docs/api/proto
COPY apps/contracts/jvm apps/contracts/jvm
COPY apps/devices apps/devices
WORKDIR /workspace/apps/devices
RUN gradle --no-daemon --max-workers=2 bootJar --write-locks
FROM eclipse-temurin:21.0.6_7-jre-alpine
WORKDIR /app
COPY --from=build /workspace/apps/devices/build/libs/devices.jar /app/devices.jar
ENTRYPOINT ["java", "-jar", "/app/devices.jar"]
