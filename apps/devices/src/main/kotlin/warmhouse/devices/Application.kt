package warmhouse.devices

import com.google.protobuf.Struct
import com.google.protobuf.Timestamp
import com.google.protobuf.util.JsonFormat
import io.grpc.Server
import io.grpc.ServerBuilder
import io.grpc.Status
import io.grpc.stub.StreamObserver
import org.slf4j.LoggerFactory
import org.springframework.boot.autoconfigure.SpringBootApplication
import org.springframework.boot.runApplication
import org.springframework.context.SmartLifecycle
import org.springframework.jdbc.core.JdbcTemplate
import org.springframework.stereotype.Component
import warmhouse.v1.*
import java.time.Instant
import java.util.concurrent.TimeUnit

@SpringBootApplication
class Application
fun main(args: Array<String>) {
    val context = runApplication<Application>(*args)
    context.getBean(DevicesRpc::class.java).awaitTermination()
}

@Component
class DevicesRpc(private val jdbc: JdbcTemplate) : DevicesServiceGrpc.DevicesServiceImplBase(), SmartLifecycle {
    private var server: Server? = null
    private val log = LoggerFactory.getLogger(javaClass)
    private val interval = (System.getenv("POLL_INTERVAL_MS") ?: "2000").toInt().also { require(it > 0) }
    private val ttl = (System.getenv("POLL_TTL_MS") ?: "1000").toInt().also { require(it > 0) }
    private val validity = (System.getenv("CONFIG_VALIDITY_MS") ?: "300000").toLong().also { require(it >= ttl) }
    private fun timestamp(t: Instant) = Timestamp.newBuilder().setSeconds(t.epochSecond).setNanos(t.nano).build()
    override fun getDevice(request: GetDeviceRequest, response: StreamObserver<Device>) {
        val rows = jdbc.queryForList("SELECT * FROM registry WHERE id=?", request.deviceId)
        if (rows.isEmpty()) { response.onError(Status.NOT_FOUND.asRuntimeException()); return }
        val row = rows.single()
        val schema = Struct.newBuilder()
        JsonFormat.parser().merge("""{"type":"object","additionalProperties":false,"properties":{"operation":{"const":"set"},"arguments":{"type":"object","additionalProperties":false,"properties":{"enabled":{"type":"boolean"}},"required":["enabled"]}},"required":["operation","arguments"]}""", schema)
        response.onNext(Device.newBuilder().setId(row["id"].toString()).setHouseId(row["house_id"].toString())
            .setTypeId(row["type_id"].toString()).setName(row["name"].toString()).setRegistrationStatus("bound")
            .addCapabilities(Capability.newBuilder().setId("cap-temperature").setCode("temperature").setKind("measurement").setValueType("number").setUnit("Cel"))
            .addCapabilities(Capability.newBuilder().setId("cap-heating").setCode("heating").setKind("control").setParametersSchema(schema)).build())
        response.onCompleted()
    }
    override fun listPollConfigurations(request: ListPollConfigurationsRequest, response: StreamObserver<PollConfigurationPage>) {
        if (request.page.limit > 500 || request.page.cursor.isNotEmpty()) { response.onError(Status.INVALID_ARGUMENT.asRuntimeException()); return }
        val id = jdbc.queryForObject("SELECT id FROM registry WHERE id='dev-thermostat'", String::class.java)!!
        val now = Instant.now()
        val config = ConfigSnapshot.newBuilder().setDeviceId(id).setCapabilityId("cap-temperature").setConfigVersion(1)
            .setAdapterKey("partner-thermostat").setEndpointKey("registered-endpoint-1").setChannel("temperature")
            .setIssuedAt(timestamp(now)).setValidUntil(timestamp(now.plusMillis(validity))).setValueType("number").setUnit("Cel")
        response.onNext(PollConfigurationPage.newBuilder().addItems(PollConfiguration.newBuilder().setConfig(config)
            .setPollIntervalMs(interval).setPollTtlMs(ttl)).build())
        response.onCompleted()
        log.info("poll configuration issued device_id={}", id)
    }
    fun awaitTermination() { server!!.awaitTermination() }
    override fun start() { server = ServerBuilder.forPort(9090).addService(this).build().start() }
    override fun stop() { server?.shutdown()?.awaitTermination(5, TimeUnit.SECONDS); server = null }
    override fun isRunning() = server != null
}
